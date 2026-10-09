package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

const ErrHistoricalCASPersistence SourceError = "historical_cas_persistence_observation_rejected"

// This is NOT a constructor for a write plan. The existing PrepareSQLCAS and
// PrepareMongoCAS gates still reject missing production authority. This seal
// only ties independently supplied actual plans to a consumed private joint
// page, the genuine four-copy EOF, and the same actual fresh origin/AI epoch.
// Its retained graphs are bounded plan baselines, not the whole coordinator.
type HistoricalCASPersistencePage struct {
	self                                                                                                        *HistoricalCASPersistencePage
	binding                                                                                                     HistoricalCoordinatorBinding
	copies                                                                                                      [4]SourceCopyReceipt
	sequence                                                                                                    uint64
	entries                                                                                                     uint64
	pageSHA, sourceSHA, originSHA, aiSHA, sqlBaseline, mongoBaseline, sqlIdentity, mongoIdentity, mongoMetadata string
	oldSQLPool                                                                                                  gorm.ConnPool
	oldMongoSession                                                                                             mongo.Session
	oldMongoTxn                                                                                                 mongoCycleTxn
	db                                                                                                          *mongo.Database
	oldSQLHead                                                                                                  string
	sql                                                                                                         *sqlevaluation.SQLHistoricalCASProvenance
	unchangedSQL                                                                                                *sqlevaluation.SQLHistoricalCASReadBaseline
	mongo                                                                                                       *MongoHistoricalBatchCASPlan
	unchangedMongo                                                                                              map[string][]bson.Raw
	expires                                                                                                     time.Time
	seal                                                                                                        string
}

type HistoricalCASAppliedPage struct {
	self              *HistoricalCASAppliedPage
	page              *HistoricalCASPersistencePage
	sql               *sqlevaluation.SQLHistoricalCASStatementBinding
	mongo             *MongoHistoricalBatchCASStatement
	writeMongoSession mongo.Session
	writeMongoTxn     mongoCycleTxn
	seal              string
}

// Exact expected persisted evidence is an observation, not proof of a Commit
// response or historical closure. This limited page proof cannot authorize any
// mutation and cannot be reconstructed from a report or JSON boolean.
type HistoricalCASPersistenceObservation struct {
	self   *HistoricalCASPersistenceObservation
	report HistoricalCASPersistenceReport
	seal   string
}

type HistoricalCASPersistenceReport struct {
	Protocol, SourceSHA, OperationID, PageSHA256, SourceCoverageSHA256              string
	SQLExpectedRowsSHA256, MongoExpectedRowsSHA256                                  string
	ObservedReferences                                                              uint64
	OriginalReadEpochEnded, ApplyEpochEnded, IndependentRawReadbackMatched          bool
	WholeFourSourceAuthenticationObserved                                           bool
	HostCommitResponseVerified, WholeRetirementPersistenceComplete                  bool
	AICommandPersistenceComplete, BusinessClosureVerified, CASAuthorized, DropReady bool
	Required                                                                        []string
}

func (*HistoricalCASPersistencePage) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCASPersistencePage) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCASPersistencePage) UnmarshalJSON([]byte) error { return ErrSourceSerialization }
func (*HistoricalCASPersistencePage) UnmarshalBSON([]byte) error { return ErrSourceSerialization }
func (*HistoricalCASPersistencePage) String() string {
	return "private CAS persistence page; no write authority"
}
func (p *HistoricalCASPersistencePage) GoString() string       { return p.String() }
func (*HistoricalCASAppliedPage) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASAppliedPage) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalCASAppliedPage) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*HistoricalCASAppliedPage) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*HistoricalCASAppliedPage) String() string {
	return "private Apply-bound page; Commit response unproven"
}
func (s *HistoricalCASAppliedPage) GoString() string { return s.String() }
func (*HistoricalCASPersistenceObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCASPersistenceObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCASPersistenceObservation) UnmarshalJSON([]byte) error {
	return ErrSourceSerialization
}
func (*HistoricalCASPersistenceObservation) UnmarshalBSON([]byte) error {
	return ErrSourceSerialization
}
func (*HistoricalCASPersistenceObservation) String() string {
	return "private limited persisted CAS observation; no DROP authority"
}
func (o *HistoricalCASPersistenceObservation) GoString() string { return o.String() }

func historicalCASObservationHash(v any) string {
	digest, err := privateFactsSHA(v)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(digest[:])
}
func (p *HistoricalCASPersistencePage) digest() string {
	if p == nil {
		return ""
	}
	pool, e := aiReversePoolToken(p.oldSQLPool)
	if e != nil {
		return ""
	}
	var sqlEntries, mongoEntries, unchangedMongo string
	if p.sql != nil {
		entries, e := p.sql.Attachments()
		if e != nil {
			return ""
		}
		sqlEntries = historicalCASObservationHash(entries)
		if sqlEntries == "" {
			return ""
		}
	}
	if p.mongo != nil {
		entries := make([]struct {
			Collection, Slot string
			ID               uint64
			Entry            evidence.HistoricalReferenceEntryV1
			Content          evidence.Digest
		}, len(p.mongo.attachments))
		for i, a := range p.mongo.attachments {
			entries[i].Collection, entries[i].Slot, entries[i].ID, entries[i].Entry, entries[i].Content = a.collection, a.slot, a.id, a.entry, a.content
		}
		mongoEntries = historicalCASObservationHash(entries)
		if mongoEntries == "" {
			return ""
		}
		groupHashes := make([]string, len(p.mongo.groups))
		for i, g := range p.mongo.groups {
			groupHashes[i] = historicalCASObservationHash(struct {
				Collection, Slot string
				ID               uint64
				PKType           uint8
				PKBytes          []byte
				Set              *evidence.HistoricalReferenceSetV1
				Entries          []evidence.HistoricalReferenceEntryV1
			}{g.collection, g.slot, g.id, uint8(g.pk.Type), g.pk.Value, g.set, g.entries})
			if groupHashes[i] == "" {
				return ""
			}
		}
		selection := historicalCASObservationHash(struct{ Sheets, Outcomes, Generations, Artifacts, Runs map[string]bool }{casPersistenceSelectorFacts(p.mongo.selection.sheets), casPersistenceSelectorFacts(p.mongo.selection.outcomes), casPersistenceSelectorFacts(p.mongo.selection.generations), casPersistenceSelectorFacts(p.mongo.selection.artifacts), casPersistenceSelectorFacts(p.mongo.selection.runs)})
		if selection == "" {
			return ""
		}
		mongoEntries = mongoOwnerHashParts(mongoEntries, mongoCASHash(p.mongo.before, p.mongo.metadataHash), p.mongo.identity, selection, historicalCASObservationHash(groupHashes))
	} else {
		unchangedMongo = mongoCASHash(p.unchangedMongo, "")
	}
	return historicalCASObservationHash(struct {
		Binding                                                                                HistoricalCoordinatorBinding
		Copies                                                                                 [4]SourceCopyReceipt
		Sequence, Entries                                                                      uint64
		Page, Sources, Origin, AI, SQL, Mongo, Head, SQLIdentity, MongoIdentity, MongoMetadata string
		Pool, MongoTxn, SQLAttachments, MongoAttachments, UnchangedMongo                       string
		Expires                                                                                time.Time
	}{p.binding, p.copies, p.sequence, p.entries, p.pageSHA, p.sourceSHA, p.originSHA, p.aiSHA, p.sqlBaseline, p.mongoBaseline, p.oldSQLHead, p.sqlIdentity, p.mongoIdentity, p.mongoMetadata, pool, mongoOwnerHashParts(string(p.oldMongoTxn.session), strconv.FormatInt(p.oldMongoTxn.number, 10)), sqlEntries, mongoEntries, unchangedMongo, p.expires})
}
func casPersistenceSelectorFacts(ids map[uint64]bool) map[string]bool {
	if ids == nil {
		return nil
	}
	facts := make(map[string]bool, len(ids))
	for id, selected := range ids {
		facts[strconv.FormatUint(id, 10)] = selected
	}
	return facts
}
func (p *HistoricalCASPersistencePage) alive(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p || p.seal == "" || p.seal != p.digest() || p.oldSQLPool == nil || p.oldMongoSession == nil || p.db == nil || !time.Now().Before(p.expires) || p.sql == nil && p.mongo == nil {
		return ErrHistoricalCASPersistence
	}
	return nil
}

func casEntryMatchesCandidate(entry evidence.HistoricalReferenceEntryV1, content evidence.Digest, candidate HistoricalCandidate, ownerID uint64, binding HistoricalCoordinatorBinding) error {
	// These private observed conditions are necessary rejection checks, never
	// a producer of write authority or a replacement for the remaining gates.
	if !candidate.LocalQualified || len(candidate.BlockingReasons) > 0 || entry.Validate() != nil || entry.EventID != candidate.OriginalID || entry.EventType != candidate.EventType || entry.Source != candidate.Source || content != candidate.ContentDigest || candidate.OwnerID != strconv.FormatUint(ownerID, 10) || entry.Proof.Verification.OperationID != binding.OperationID {
		return ErrHistoricalCASPersistence
	}
	if len(candidate.HistoricalGaps) > 0 && entry.Proof.Class != evidence.Unverifiable {
		return ErrHistoricalCASPersistence
	}
	if candidate.BusinessBindingSHA256 != "" && entry.Proof.BusinessBindingSHA256 != candidate.BusinessBindingSHA256 {
		return ErrHistoricalCASPersistence
	}
	// Original Run may be absent by the old source contract. Never fill it from
	// the current/latest Run, or convert an observation to terminal approval.
	if !reflect.DeepEqual(entry.Run, candidate.ActualOriginalRun) {
		return ErrHistoricalCASPersistence
	}
	return nil
}

func (c *HistoricalCoordinator) SealHistoricalCASPersistencePage(ctx context.Context, joint *WholeSourceJointPage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof, sqlPlan *sqlevaluation.SQLHistoricalBatchCASPlan, mongoPlan *MongoHistoricalBatchCASPlan) (*HistoricalCASPersistencePage, error) {
	if c == nil || ctx == nil || joint == nil || origin == nil || origin.second == nil || ai == nil || ai.self != ai || ai.anchor == nil || sqlPlan == nil && mongoPlan == nil {
		return nil, ErrHistoricalCASPersistence
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || !c.coverage || c.failed || c.authenticated == nil || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || len(c.remaining) != 0 || c.receipts != c.authenticated.receipts || joint.owner != c || !joint.consumed || joint.page == nil || joint.index == nil || joint.index.auth != c.authenticated || joint.candidateStart < 0 || joint.candidateCount <= 0 || joint.candidateCount > 512 || joint.candidateStart+joint.candidateCount > len(c.candidates) || !coordinatorStoredCandidateMatches(c.candidates[joint.candidateStart:joint.candidateStart+joint.candidateCount], joint.candidateSHA) {
		return nil, ErrHistoricalCASPersistence
	}
	if joint.ValidateBorrowedSnapshot(ctx) != nil || origin.second.validate(ctx) != nil || origin.second.binding.copies != c.authenticated || origin.second.sql != joint.sql.responsibility.cycle || origin.second.mongo != joint.mongo.global || ai.current != joint.sql.responsibility || ai.coordinator != c || ai.currentCycle != joint.sql.responsibility.Report().CycleID || ai.anchor.alive(ctx) != nil || ai.anchor.oldTransactionEnded(ctx) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || ai.currentPool != tx.Statement.ConnPool || origin.second.sqlConnection != tx.Statement.ConnPool {
		return nil, ErrHistoricalCASPersistence
	}
	session := mongo.SessionFromContext(ctx)
	actual, err := mongoCycleTransaction(ctx, joint.mongo.global.db)
	if err != nil || session == nil || !bytes.Equal(actual.session, joint.mongo.global.txn.session) || actual.number != joint.mongo.global.txn.number {
		return nil, ErrHistoricalCASPersistence
	}
	p := &HistoricalCASPersistencePage{binding: c.binding, copies: c.receipts, sequence: joint.page.sequence, entries: uint64(joint.candidateCount), pageSHA: joint.candidateSHA, sourceSHA: historicalCASObservationHash(c.receipts), originSHA: origin.second.hash, aiSHA: ai.classificationSHA, sqlBaseline: joint.sql.Report().BusinessRowsSHA256, mongoBaseline: joint.mongo.Report().BusinessRowsSHA256, sqlIdentity: joint.sql.Report().DatabaseIdentitySHA256, mongoIdentity: joint.mongo.global.metadata.identity, mongoMetadata: joint.mongo.global.metadata.hash, oldSQLPool: tx.Statement.ConnPool, oldMongoSession: session, oldMongoTxn: actual, db: joint.mongo.global.db, oldSQLHead: origin.second.sqlHead, expires: c.started.Add(c.limits.MaxDuration)}
	if joint.mongo.started.Add(joint.mongo.limits.MaxDuration).Before(p.expires) {
		p.expires = joint.mongo.started.Add(joint.mongo.limits.MaxDuration)
	}
	// Preserve every inherited absolute budget. Sealing/rebinding must never
	// turn the old origin/AI/coordinator qualification into a new TTL.
	for _, deadline := range []time.Time{origin.second.binding.started.Add(origin.second.binding.limits.MaxDuration), ai.anchor.started.Add(ai.anchor.limits.MaxDuration), ai.anchor.coordinatorStarted.Add(ai.anchor.coordinatorDuration)} {
		if deadline.Before(p.expires) {
			p.expires = deadline
		}
	}
	// Exact source-key coverage is private and bounded to this original page.
	byKey := map[verifiedSourceKey]HistoricalCandidate{}
	for _, candidate := range c.candidates[joint.candidateStart : joint.candidateStart+joint.candidateCount] {
		key, e := sourceAuthKey(candidate.Source.Database, candidate.Source.Object, candidate.Source.PrimaryKeySHA256)
		if e != nil || byKey[key].OriginalID != "" {
			return nil, ErrHistoricalCASPersistence
		}
		byKey[key] = coordinatorCloneCandidate(*candidate)
	}
	seen := map[verifiedSourceKey]bool{}
	match := func(entry evidence.HistoricalReferenceEntryV1, content evidence.Digest, id uint64) error {
		key, e := sourceAuthKey(entry.Source.Database, entry.Source.Object, entry.Source.PrimaryKeySHA256)
		candidate, ok := byKey[key]
		if e != nil || !ok || seen[key] || casEntryMatchesCandidate(entry, content, candidate, id, c.binding) != nil {
			return ErrHistoricalCASPersistence
		}
		seen[key] = true
		return nil
	}
	if sqlPlan != nil {
		p.sql, err = sqlevaluation.SealSQLHistoricalCASProvenance(ctx, sqlPlan, joint.sql.facts)
		if err != nil {
			return nil, err
		}
		attachments, e := p.sql.Attachments()
		if e != nil {
			return nil, e
		}
		for _, a := range attachments {
			id := a.AssessmentID
			if a.OutcomeID != 0 {
				id = a.OutcomeID
			}
			if match(a.Entry, a.ContentDigest, id) != nil {
				return nil, ErrHistoricalCASPersistence
			}
		}
	} else {
		p.unchangedSQL, err = sqlevaluation.SealSQLHistoricalCASReadBaseline(ctx, joint.sql.facts)
		if err != nil {
			return nil, err
		}
	}
	if mongoPlan != nil {
		b := joint.mongo
		if mongoPlan.db != b.global.db || mongoPlan.oldTxn.number != b.global.txn.number || !bytes.Equal(mongoPlan.oldTxn.session, b.global.txn.session) || mongoPlan.metadataHash != b.global.metadata.hash || mongoPlan.identity != b.global.metadata.identity || mongoPlan.originalSQLConnection != b.sqlConnection || mongoPlan.originalSQLIdentity != b.sql.Report().DatabaseIdentitySHA256 || mongoPlan.originalSQLRows != b.sql.Report().BusinessRowsSHA256 || !reflect.DeepEqual(mongoPlan.before, b.data) || !reflect.DeepEqual(mongoPlan.selection, b.selection) || len(mongoPlan.attachments) == 0 {
			return nil, ErrHistoricalCASPersistence
		}
		for _, a := range mongoPlan.attachments {
			if match(a.entry, a.content, a.id) != nil {
				return nil, ErrHistoricalCASPersistence
			}
		}
		p.mongo = mongoPlan
		if mongoPlan.expires.Before(p.expires) {
			p.expires = mongoPlan.expires
		}
	} else {
		p.unchangedMongo = mongoCASCloneData(joint.mongo.data)
	}
	if len(seen) != len(byKey) || joint.ValidateBorrowedSnapshot(ctx) != nil || c.alive(ctx) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	p.self, p.seal = p, p.digest()
	if p.alive(ctx) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	return p, nil
}

func casPersistenceSQLPoolEnded(ctx context.Context, pool gorm.ConnPool) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrHistoricalCASPersistence
	}
	if _, err := aiReversePoolToken(pool); err != nil {
		return ErrHistoricalCASPersistence
	}
	scope, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := pool.QueryContext(scope, "SELECT 1")
	if rows != nil {
		if e := rows.Close(); e != nil {
			return ErrHistoricalCASPersistence
		}
	}
	if scope.Err() != nil || !errors.Is(err, sql.ErrTxDone) {
		return ErrHistoricalCASPersistence
	}
	return nil
}

// The pinned actual Session's transaction counter/state proves only that the
// original transaction ended. Same Lsid with a genuinely newer Txn is allowed.
func casPersistenceMongoEpochEnded(session mongo.Session, old mongoCycleTxn) error {
	if session == nil {
		return ErrHistoricalCASPersistence
	}
	v := reflect.ValueOf(session)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return ErrHistoricalCASPersistence
	}
	x, ok := session.(mongo.XSession) //nolint:staticcheck // pinned actual transaction accessor
	if !ok || x.ClientSession() == nil || !bytes.Equal(session.ID(), old.session) {
		return ErrHistoricalCASPersistence
	}
	actual := x.ClientSession()
	if actual.TxnNumber < old.number || actual.TxnNumber == old.number && actual.TransactionRunning() {
		return ErrHistoricalCASPersistence
	}
	return nil
}

// Call after actual Apply, in its still-live borrowed RW epoch, before host
// Commit/Rollback. This method does not call either Apply or any lifecycle API.
func (p *HistoricalCASPersistencePage) BindApplied(ctx context.Context, sqlStatement *sqlevaluation.SQLHistoricalBatchCASStatement, mongoStatement *MongoHistoricalBatchCASStatement) (*HistoricalCASAppliedPage, error) {
	if p.alive(ctx) != nil || (p.sql == nil) != (sqlStatement == nil) || (p.mongo == nil) != (mongoStatement == nil) || casPersistenceSQLPoolEnded(ctx, p.oldSQLPool) != nil || casPersistenceMongoEpochEnded(p.oldMongoSession, p.oldMongoTxn) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	s := &HistoricalCASAppliedPage{page: p}
	if p.sql != nil {
		var err error
		s.sql, err = p.sql.BindStatement(ctx, sqlStatement)
		if err != nil {
			return nil, err
		}
	}
	if p.mongo != nil {
		if mongoStatement.plan != p.mongo {
			return nil, ErrHistoricalCASPersistence
		}
		actual, err := mongoCycleTransaction(ctx, p.db)
		if err != nil || actual.number != mongoStatement.transaction.number || !bytes.Equal(actual.session, mongoStatement.transaction.session) || actual.number == p.oldMongoTxn.number && bytes.Equal(actual.session, p.oldMongoTxn.session) {
			return nil, ErrHistoricalCASPersistence
		}
		expected, err := p.mongo.expectedData()
		if err != nil || !reflect.DeepEqual(expected, mongoStatement.expected) {
			return nil, ErrHistoricalCASPersistence
		}
		s.mongo, s.writeMongoSession, s.writeMongoTxn = mongoStatement, mongo.SessionFromContext(ctx), actual
	}
	s.self = s
	s.seal = s.digest()
	if !s.intact() {
		return nil, ErrHistoricalCASPersistence
	}
	return s, nil
}
func (s *HistoricalCASAppliedPage) digest() string {
	if s == nil || s.page == nil {
		return ""
	}
	var sqlRows, mongoRows string
	if s.sql != nil {
		sqlRows = s.sql.StatementReport().ExpectedBusinessRowsSHA256
	}
	if s.mongo != nil {
		mongoRows = mongoCASHash(s.mongo.expected, s.page.mongo.metadataHash)
	}
	return historicalCASObservationHash(struct{ Page, SQL, Mongo string }{s.page.seal, sqlRows, mongoRows})
}
func (s *HistoricalCASAppliedPage) intact() bool {
	return s != nil && s.self == s && s.page != nil && s.seal != "" && s.seal == s.digest()
}

// The HOST starts/ends all scopes. Exact persisted expected baselines are
// observed only in a third genuine SQL RRRO/Mongo snapshot; ended write scopes
// cannot certify whether Commit returned success. No DTO controls this path.
func (s *HistoricalCASAppliedPage) VerifyPersisted(ctx context.Context, sqlFresh *SQLBusinessOwnerBatch, mongoFresh *MongoHistoricalOwnerBatch) (*HistoricalCASPersistenceObservation, error) {
	if !s.intact() || s.page.alive(ctx) != nil || sqlFresh == nil || mongoFresh == nil || mongoFresh.sql != sqlFresh.facts || casPersistenceSQLPoolEnded(ctx, s.page.oldSQLPool) != nil || casPersistenceMongoEpochEnded(s.page.oldMongoSession, s.page.oldMongoTxn) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	if sqlFresh.ValidateBorrowedSnapshot(ctx) != nil || mongoFresh.ValidateBorrowedSnapshot(ctx) != nil || sqlFresh.Report().DatabaseIdentitySHA256 != s.page.sqlIdentity || mongoFresh.global.metadata.identity != s.page.mongoIdentity || mongoFresh.global.metadata.hash != s.page.mongoMetadata {
		return nil, ErrHistoricalCASPersistence
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == s.page.oldSQLPool {
		return nil, ErrHistoricalCASPersistence
	}
	actual, err := mongoCycleTransaction(ctx, mongoFresh.global.db)
	if err != nil || actual.number == s.page.oldMongoTxn.number && bytes.Equal(actual.session, s.page.oldMongoTxn.session) {
		return nil, ErrHistoricalCASPersistence
	}
	var sqlRows, mongoRows string
	if s.sql != nil {
		if err = s.sql.VerifyPersisted(ctx, sqlFresh.facts); err != nil {
			return nil, err
		}
		sqlRows = s.sql.StatementReport().ExpectedBusinessRowsSHA256
	} else {
		if s.page.unchangedSQL == nil || s.page.unchangedSQL.VerifyUnchanged(ctx, sqlFresh.facts) != nil {
			return nil, ErrHistoricalCASPersistence
		}
		sqlRows = sqlFresh.Report().BusinessRowsSHA256
	}
	if s.mongo != nil {
		if casPersistenceMongoEpochEnded(s.writeMongoSession, s.writeMongoTxn) != nil {
			return nil, ErrHistoricalCASPersistence
		}
		if err = s.mongo.VerifyPersisted(ctx, mongoFresh); err != nil {
			return nil, err
		}
		mongoRows = mongoCASHash(s.mongo.expected, s.page.mongo.metadataHash)
	} else {
		if !reflect.DeepEqual(mongoFresh.data, s.page.unchangedMongo) {
			return nil, ErrHistoricalCASPersistence
		}
		mongoRows = mongoFresh.Report().BusinessRowsSHA256
	}
	if !s.intact() || sqlFresh.ValidateBorrowedSnapshot(ctx) != nil || mongoFresh.ValidateBorrowedSnapshot(ctx) != nil || s.page.alive(ctx) != nil {
		return nil, ErrHistoricalCASPersistence
	}
	r := historicalCASPersistenceEmptyReport()
	r.SourceSHA, r.OperationID, r.PageSHA256, r.SourceCoverageSHA256 = s.page.binding.SourceSHA, s.page.binding.OperationID, s.page.pageSHA, s.page.sourceSHA
	r.SQLExpectedRowsSHA256, r.MongoExpectedRowsSHA256 = sqlRows, mongoRows
	r.ObservedReferences = s.page.entries
	r.OriginalReadEpochEnded, r.ApplyEpochEnded, r.IndependentRawReadbackMatched, r.WholeFourSourceAuthenticationObserved = true, true, true, true
	o := &HistoricalCASPersistenceObservation{report: r}
	o.self = o
	o.seal = historicalCASObservationHash(r)
	if o.seal == "" {
		return nil, ErrHistoricalCASPersistence
	}
	return o, nil
}
func historicalCASPersistenceEmptyReport() HistoricalCASPersistenceReport {
	return HistoricalCASPersistenceReport{Protocol: "historical-cas-persistence-page-observation/v1", Required: []string{"independent_production_source_approval", "original_joint_business_and_external_qs_ai_closure", "whole_writer_and_historical_rerun_fence", "host_commit_response_authentication", "ai_command_retirement_and_independent_readback", "whole_four_source_persistence_coverage", "post_cas_expected_global_and_absent_range_fresh_recheck"}}
}
func (o *HistoricalCASPersistenceObservation) Report() HistoricalCASPersistenceReport {
	if o == nil || o.self != o || o.seal == "" || o.seal != historicalCASObservationHash(o.report) {
		return historicalCASPersistenceEmptyReport()
	}
	r := o.report
	r.Required = append([]string(nil), o.report.Required...)
	return r
}
