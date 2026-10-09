package retirement

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WholeSourceJointReplayAnchor preserves a consumed page's original epoch and
// body-free receipt. It retains no joint, business batch, source handle or graph.
// It is not a source approval, fresh proof, write authorization or DROP proof.
// The coordinator and host-owned transaction/session remain borrowed.
type WholeSourceJointReplayAnchor struct {
	self                                               *WholeSourceJointReplayAnchor
	owner                                              *HistoricalCoordinator
	binding                                            HistoricalCoordinatorBinding
	receipt                                            HistoricalCoordinatorPageReceipt
	candidateOffset, candidateCount                    int
	indexSHA, expectedSHA, bindingSHA, crossRowsSHA    string
	encodedSHA                                         [4]string
	copies                                             [4]SourceCopyReceipt
	sqlPool                                            gorm.ConnPool
	sqlCycle, sqlReportSHA, catalogSHA, mongoReportSHA string
	mongoSession                                       mongo.Session
	mongoTxn                                           mongoCycleTxn
	db                                                 *mongo.Database
	mongoStarted                                       time.Time
	mongoLimits                                        MongoHistoricalOwnerBatchLimits
	started, expires                                   time.Time
	seal                                               string
}

func (*WholeSourceJointReplayAnchor) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*WholeSourceJointReplayAnchor) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*WholeSourceJointReplayAnchor) UnmarshalJSON([]byte) error { return ErrSourceSerialization }
func (*WholeSourceJointReplayAnchor) UnmarshalBSON([]byte) error { return ErrSourceSerialization }
func (*WholeSourceJointReplayAnchor) String() string {
	return "private consumed-page replay anchor; no fresh or execution authority"
}
func (a *WholeSourceJointReplayAnchor) GoString() string { return a.String() }

func (a *WholeSourceJointReplayAnchor) digest() string {
	if a == nil {
		return ""
	}
	pool, err := aiReversePoolToken(a.sqlPool)
	if err != nil {
		return ""
	}
	return historicalCASObservationHash(struct {
		Binding                                                                              HistoricalCoordinatorBinding
		Receipt                                                                              HistoricalCoordinatorPageReceipt
		Offset, Count                                                                        int
		Index, Expected, BindingSHA, CrossRows, Pool, Cycle, SQLReport, Catalog, MongoReport string
		Encoded                                                                              [4]string
		Copies                                                                               [4]SourceCopyReceipt
		MongoSession                                                                         bson.Raw
		MongoTransaction                                                                     int64
		MongoLimits                                                                          MongoHistoricalOwnerBatchLimits
		Started, MongoStarted, Expires                                                       time.Time
	}{a.binding, a.receipt, a.candidateOffset, a.candidateCount, a.indexSHA, a.expectedSHA, a.bindingSHA, a.crossRowsSHA, pool, a.sqlCycle, a.sqlReportSHA, a.catalogSHA, a.mongoReportSHA, a.encodedSHA, a.copies, a.mongoTxn.session, a.mongoTxn.number, a.mongoLimits, a.started, a.mongoStarted, a.expires})
}

func wholeJointReplayExpectedSHA(c *HistoricalCoordinator) string {
	var expected [4]SourceCopyExpectation
	for i := range expected {
		expected[i] = c.copies[i].Expected
	}
	return historicalCASObservationHash(expected)
}

// The receipt must select exactly this stored private candidate block. The
// sequence/offset are selectors, never caller-provided completion assertions.
// Caller holds c.mu; this helper does not mint a replay or any write capability.
func wholeJointReplayCandidatesLocked(c *HistoricalCoordinator, sequence uint64, offset, count int, expected HistoricalCoordinatorPageReceipt) ([]HistoricalCandidate, error) {
	if c == nil || c.authenticated == nil || sequence == 0 || sequence > uint64(len(c.pages)) || count <= 0 || count > c.limits.MaxPageRecords || count > 512 || offset < 0 || offset > len(c.candidates) || count > len(c.candidates)-offset || expected.Sequence != sequence || c.pages[sequence-1] != expected || expected.PageTTLMilliseconds != c.limits.PageTTL.Milliseconds() || expected.IssuedAt.IsZero() || expected.ConsumedAt.Before(expected.IssuedAt) || expected.ConsumedAt.Sub(expected.IssuedAt) > c.limits.PageTTL+time.Millisecond {
		return nil, ErrCoordinatorPage
	}
	// Receipt clocks were truncated independently to milliseconds; the extra
	// millisecond above only accounts for that representation. Actual consume
	// already enforced the original untruncated PageTTL.
	// The self-bound replay anchor fixes the original joint's actual offset.
	// consume already enforces global physical source-key uniqueness; the
	// exact stored block/receipt hash below must match that original page.
	// Rewalking every preceding page here would make full replay quadratic.
	if !coordinatorStoredCandidateMatches(c.candidates[offset:offset+count], expected.CandidateSHA256) {
		return nil, ErrCoordinatorPage
	}
	out := make([]HistoricalCandidate, count)
	observed := expected
	observed.Records, observed.FirstPrimaryKeyBySourceSHA256, observed.LastPrimaryKeyBySourceSHA256 = [4]uint64{}, [4]string{}, [4]string{}
	seen := make(map[verifiedSourceKey]bool, count)
	for i, candidate := range c.candidates[offset : offset+count] {
		if candidate == nil {
			return nil, ErrCoordinatorPage
		}
		key, err := sourceAuthKey(candidate.Source.Database, candidate.Source.Object, candidate.Source.PrimaryKeySHA256)
		if err != nil || key.object != 0 && key.object != 3 || seen[key] {
			return nil, ErrCoordinatorPage
		}
		if _, ok := c.authenticated.rows[key]; !ok {
			return nil, ErrSourceAuthentication
		}
		seen[key] = true
		if observed.FirstPrimaryKeyBySourceSHA256[key.object] == "" {
			observed.FirstPrimaryKeyBySourceSHA256[key.object] = candidate.Source.PrimaryKeySHA256
		}
		observed.LastPrimaryKeyBySourceSHA256[key.object] = candidate.Source.PrimaryKeySHA256
		observed.Records[key.object]++
		out[i] = wholeJointReplayCloneCandidate(*candidate)
	}
	observed.FirstPrimaryKeySHA256, observed.LastPrimaryKeySHA256 = out[0].Source.PrimaryKeySHA256, out[len(out)-1].Source.PrimaryKeySHA256
	observed.CandidateSHA256 = coordinatorCandidateHash(out)
	if observed != expected {
		return nil, ErrCoordinatorPage
	}
	return out, nil
}

// Keep nil versus non-nil empty profiles exactly as recorded by consume.
// Ordinary public candidate cloning intentionally normalizes empty lists;
// replay compares the original private typed digest and must not normalize.
func wholeJointReplayCloneCandidate(value HistoricalCandidate) HistoricalCandidate {
	result := coordinatorCloneCandidate(value)
	clone := func(values []string) []string {
		if values == nil {
			return nil
		}
		out := make([]string, len(values))
		copy(out, values)
		return out
	}
	result.OriginalRunMissing = clone(value.OriginalRunMissing)
	result.HistoricalGaps = clone(value.HistoricalGaps)
	result.BlockingReasons = clone(value.BlockingReasons)
	result.RequiredAdapters = clone(value.RequiredAdapters)
	return result
}

// Seal immediately after real joint consumption, before releasing that graph.
// Sealing before coordinator EOF grants no replay: replay independently requires
// the coordinator's full second four-source pass and exactly-once coverage.
func (c *HistoricalCoordinator) SealWholeSourceJointReplayAnchor(ctx context.Context, joint *WholeSourceJointPage) (*WholeSourceJointReplayAnchor, error) {
	if c == nil || ctx == nil || joint == nil {
		return nil, ErrWholeSourceJoint
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if joint.owner != c || !joint.consumed || joint.page == nil || joint.page.owner != c || !joint.page.consumed || joint.page.sequence == 0 || joint.page.sequence > uint64(len(c.pages)) || joint.index == nil || !joint.index.complete || joint.index.owner != c || joint.index.auth != c.authenticated || joint.catalog == nil || joint.catalog.current == nil || joint.catalog.index == nil || joint.sql == nil || joint.sql.facts == nil || joint.mongo == nil || joint.cross == nil || joint.sql.responsibility != joint.catalog.current || joint.mongo.global == nil {
		return nil, ErrWholeSourceJoint
	}
	receipt := c.pages[joint.page.sequence-1]
	values, err := wholeJointReplayCandidatesLocked(c, joint.page.sequence, joint.candidateStart, joint.candidateCount, receipt)
	if err != nil {
		return nil, err
	}
	if joint.index.receipts != c.authenticated.receipts || !joint.catalog.Report().Complete || !joint.sql.Report().Complete || !joint.mongo.Report().SourceCopyFactsBound || len(joint.current) != len(values) || joint.candidateSHA != receipt.CandidateSHA256 || joint.sql.Report().BusinessRowsSHA256 != receipt.SQLBusinessBaselineSHA256 || joint.mongo.Report().BusinessRowsSHA256 != receipt.MongoBusinessBaselineSHA256 {
		return nil, ErrWholeSourceJoint
	}
	binding, err := wholeSourceJointBindingsHash(joint.bindings)
	if err != nil || binding != joint.bindingSHA || !wholeJointReplayValuesMatch(ctx, joint, values) {
		return nil, ErrWholeSourceJoint
	}
	if err = joint.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Statement.ConnPool != joint.mongo.sqlConnection {
		return nil, ErrWholeSourceJoint
	}
	session := mongo.SessionFromContext(ctx)
	txn, err := mongoCycleTransaction(ctx, joint.mongo.global.db)
	if err != nil || session == nil || txn.number != joint.mongo.global.txn.number || !bytes.Equal(txn.session, joint.mongo.global.txn.session) {
		return nil, ErrWholeSourceJoint
	}
	a := &WholeSourceJointReplayAnchor{owner: c, binding: c.binding, receipt: receipt, candidateOffset: joint.candidateStart, candidateCount: joint.candidateCount, indexSHA: joint.index.indexSHA, expectedSHA: wholeJointReplayExpectedSHA(c), bindingSHA: binding, crossRowsSHA: joint.cross.Report().RowsSHA256, encodedSHA: joint.index.encodedSHA, copies: c.authenticated.receipts, sqlPool: tx.Statement.ConnPool, sqlCycle: joint.sql.responsibility.Report().CycleID, sqlReportSHA: historicalCASObservationHash(joint.sql.responsibility.Report()), catalogSHA: historicalCASObservationHash(joint.catalog.Report()), mongoReportSHA: historicalCASObservationHash(joint.mongo.global.Report()), mongoSession: session, mongoTxn: mongoCycleTxn{session: append(bson.Raw(nil), txn.session...), number: txn.number}, db: joint.mongo.global.db, mongoStarted: joint.mongo.started, mongoLimits: joint.mongo.limits, started: c.started, expires: c.started.Add(c.limits.MaxDuration)}
	for _, deadline := range []time.Time{joint.mongo.started.Add(joint.mongo.limits.MaxDuration), joint.mongo.global.started.Add(joint.mongo.global.limits.MaxDuration)} {
		if deadline.Before(a.expires) {
			a.expires = deadline
		}
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(a.expires) {
		a.expires = deadline
	}
	a.self, a.seal = a, a.digest()
	if a.seal == "" || !time.Now().Before(a.expires) {
		return nil, ErrCoordinatorExpired
	}
	return a, nil
}

func wholeJointReplayValuesMatch(ctx context.Context, joint *WholeSourceJointPage, expected []HistoricalCandidate) bool {
	if joint == nil || len(joint.current) != len(expected) {
		return false
	}
	actual := make([]HistoricalCandidate, len(expected))
	for i, handle := range joint.current {
		value, err := joint.Candidate(ctx, handle)
		if err != nil {
			return false
		}
		actual[i] = value
	}
	return coordinatorCandidateHash(actual) == coordinatorCandidateHash(expected)
}

// Caller holds c.mu. A complete public receipt is never accepted in place of
// this actual coordinator's own complete authenticated/consumed source state.
func (a *WholeSourceJointReplayAnchor) candidatesLocked(ctx context.Context, c *HistoricalCoordinator, sequence uint64, offset int, index *WholeSourceJointIndex) ([]HistoricalCandidate, error) {
	if c == nil || a == nil || a.self != a || a.owner != c || ctx == nil || a.seal == "" || a.digest() != a.seal || a.binding != c.binding || !a.started.Equal(c.started) || sequence != a.receipt.Sequence || offset != a.candidateOffset {
		return nil, ErrWholeSourceJoint
	}
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if !time.Now().Before(a.expires) {
		return nil, ErrCoordinatorExpired
	}
	if !c.coverage || c.pending != nil || len(c.remaining) != 0 || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || c.authenticated.entries != uint64(len(c.candidates)) || c.receipts != c.authenticated.receipts || c.receipts != a.copies {
		return nil, ErrCoordinatorIncomplete
	}
	for i := range c.consumed {
		if c.consumed[i] != c.receipts[i].Records {
			return nil, ErrCoordinatorIncomplete
		}
	}
	if index == nil || !index.complete || index.owner != c || index.auth != c.authenticated || index.indexSHA != a.indexSHA || index.encodedSHA != a.encodedSHA || index.receipts != a.copies || wholeJointReplayExpectedSHA(c) != a.expectedSHA {
		return nil, ErrSourceAuthentication
	}
	return wholeJointReplayCandidatesLocked(c, sequence, offset, a.candidateCount, a.receipt)
}

func (a *WholeSourceJointReplayAnchor) validateEpoch(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, global *MongoResponsibilitySnapshot) error {
	if ctx == nil || a == nil || a.self != a || catalog == nil || catalog.current == nil || catalog.index == nil || global == nil || global.db != a.db || mongo.SessionFromContext(ctx) != a.mongoSession || catalog.current.Report().CycleID != a.sqlCycle || historicalCASObservationHash(catalog.current.Report()) != a.sqlReportSHA || historicalCASObservationHash(catalog.Report()) != a.catalogSHA || historicalCASObservationHash(global.Report()) != a.mongoReportSHA {
		return ErrWholeSourceJoint
	}
	if err := catalog.current.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	if err := global.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Statement.ConnPool != a.sqlPool {
		return ErrWholeSourceJoint
	}
	txn, err := mongoCycleTransaction(ctx, a.db)
	if err != nil || txn.number != a.mongoTxn.number || !bytes.Equal(txn.session, a.mongoTxn.session) {
		return ErrWholeSourceJoint
	}
	return nil
}

// ReplayWholeSourceJointPage rereads the original ReaderAt frames and original
// bounded business/owner graphs in the SAME borrowed RR-RO/snapshot epoch.
// It changes no source cursor, receipt, candidate, count, sequence or deadline.
// This is not RecheckFresh and cannot survive a baseline change after a CAS.
func (c *HistoricalCoordinator) ReplayWholeSourceJointPage(ctx context.Context, anchor *WholeSourceJointReplayAnchor, sequence uint64, candidateOffset int, index *WholeSourceJointIndex, catalog *SQLCrossStoreResponsibilityCatalog, global *MongoResponsibilitySnapshot) (*WholeSourceJointPage, error) {
	if c == nil {
		return nil, ErrWholeSourceJoint
	}
	c.mu.Lock()
	values, err := anchor.candidatesLocked(ctx, c, sequence, candidateOffset, index)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// Inherit one absolute deadline and the same actual SessionContext. The
	// component neither creates a session nor changes either transaction.
	scope, cancel := context.WithDeadline(ctx, anchor.expires)
	defer cancel()
	ctx = mongo.NewSessionContext(scope, mongo.SessionFromContext(ctx))
	if err = anchor.validateEpoch(ctx, catalog, global); err != nil {
		return nil, err
	}
	current, rows, err := wholeJointReplaySourceRows(ctx, index, values)
	if err != nil {
		return nil, err
	}
	request, err := MongoHistoricalSQLBatchSelectors(current)
	if err != nil {
		return nil, err
	}
	initial, err := PrepareSQLBusinessOwnerBatch(ctx, catalog.current, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
	if err != nil {
		return nil, err
	}
	// Rows are private, freshly authenticated handles; consumed remains true so
	// neither Events nor a qualification call can re-consume this source page.
	page := &HistoricalSourcePage{owner: c, sequence: sequence, issued: anchor.receipt.IssuedAt, rows: rows, consumed: true}
	joint, err := prepareWholeSourceJointPage(ctx, c, page, index, catalog, initial, global, current)
	if err != nil {
		return nil, err
	}
	// The lower-level builder timestamps its temporary read batch. Restore the
	// inherited absolute owner-page budget before returning any capability.
	// Do not reopen the original consumption PageTTL or extend the whole epoch.
	if joint.mongo.limits != anchor.mongoLimits {
		return nil, ErrWholeSourceJoint
	}
	joint.mongo.started = anchor.mongoStarted
	if remaining := anchor.expires.Sub(anchor.mongoStarted); remaining < joint.mongo.limits.MaxDuration {
		joint.mongo.limits.MaxDuration = remaining
	}
	if joint.bindingSHA != anchor.bindingSHA || joint.cross.Report().RowsSHA256 != anchor.crossRowsSHA || joint.sql.Report().BusinessRowsSHA256 != anchor.receipt.SQLBusinessBaselineSHA256 || joint.mongo.Report().BusinessRowsSHA256 != anchor.receipt.MongoBusinessBaselineSHA256 || !wholeJointReplayValuesMatch(ctx, joint, values) {
		return nil, ErrSQLMongoCrossStoreConflict
	}
	if err = joint.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	if err = anchor.validateEpoch(ctx, catalog, global); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	final, err := anchor.candidatesLocked(ctx, c, sequence, candidateOffset, index)
	if err != nil {
		return nil, err
	}
	if coordinatorCandidateHash(final) != coordinatorCandidateHash(values) {
		return nil, ErrCoordinatorPage
	}
	joint.candidateStart, joint.candidateCount, joint.candidateSHA, joint.consumed = candidateOffset, len(values), anchor.receipt.CandidateSHA256, true
	return joint, nil
}

// No editable source DTO can supply these rows: index.event independently
// verifies the original physical frame, decoded facts and authenticated clone.
func wholeJointReplaySourceRows(ctx context.Context, index *WholeSourceJointIndex, values []HistoricalCandidate) ([]*VerifiedSourceEvent, []coordinatorRow, error) {
	if ctx == nil || index == nil || len(values) == 0 || len(values) > 512 {
		return nil, nil, ErrCoordinatorPage
	}
	current := make([]*VerifiedSourceEvent, 0, len(values))
	rows := make([]coordinatorRow, 0, len(values))
	for _, value := range values {
		handle, err := index.event(ctx, value.OriginalID)
		if err != nil {
			return nil, nil, err
		}
		facts, err := handle.Facts()
		if err != nil {
			return nil, nil, err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		entry, ok := index.entries[value.OriginalID]
		if err != nil || !ok || key != entry.Key || !reflect.DeepEqual(facts.Source, value.Source) || facts.EventID != value.OriginalID || facts.EventType != value.EventType || strconv.FormatUint(facts.OrgID, 10) != value.OrganizationID || !reflect.DeepEqual(facts.ContentDigest, value.ContentDigest) {
			return nil, nil, ErrSourceAuthentication
		}
		current = append(current, handle)
		rows = append(rows, coordinatorRow{event: handle, keys: []verifiedSourceKey{key}, bytes: uint64(entry.Length)})
	}
	return current, rows, nil
}
