package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const sourceOriginInputPageBytes = 64 << 20

// HistoricalSourceInputRecipe freezes authenticated input, never the old
// origin/global capability or its lifetime. Only the live binding can mint it.
// The host retains ownership of every original stream and all later scopes.
type HistoricalSourceInputRecipe struct {
	self           *HistoricalSourceInputRecipe
	binding        OriginCopyBinding
	hash           string
	captureStopped bool
}

type sourceOriginInputFrame struct {
	Version, Source  int
	Recipe, Epoch    string
	Boundary         SourceBoundary
	Columns, SQLRows SQLColumns
	MongoRows        []bson.Raw
	EOF              bool
}
type sourceOriginInputSink func(context.Context, sourceOriginInputFrame) error

// The disk form uses explicit JSON NULL cells and base64 source bytes: gob
// cannot represent nil pointers inside SQL cell slices without losing NULL.
type sourceOriginInputDiskFrame struct {
	Version, Source          int
	Recipe, Epoch            string
	Boundary                 SourceBoundary
	ColumnsJSON, SQLRowsJSON []byte
	MongoRows                []bson.Raw
	EOF                      bool
}

// HistoricalSourceInputEpoch is an actual four-source read, with raw pages in
// a host-supplied private spool. It is not accepted by any legacy qualification
// or writing API. Session, transaction, connection and file remain host owned.
type HistoricalSourceInputEpoch struct {
	self                                                           *HistoricalSourceInputEpoch
	recipe                                                         *HistoricalSourceInputRecipe
	sql                                                            *sqlevaluation.SQLHistoricalResponsibilityCycle
	mongo                                                          *MongoSnapshotInputEpoch
	sqlConnection                                                  any
	sqlCycleID, sqlIdentity, sqlHead, mongoIdentity, mongoMetadata string
	mongoSession                                                   mongo.Session
	mongoSessionID                                                 bson.Raw
	mongoTime                                                      primitive.Timestamp
	started                                                        time.Time
	budget                                                         time.Duration
	file                                                           *os.File
	dev, ino                                                       uint64
	end                                                            int64
	pages                                                          []historicalSpoolRef
	receipts                                                       [4]SourceCopyReceipt
	boundaries                                                     [4]SourceBoundary
	epochHash, resultHash                                          string
	complete, poisoned                                             bool
	captureStopped                                                 bool
}

type HistoricalSourceInputPair struct {
	self          *HistoricalSourceInputPair
	first, second *HistoricalSourceInputEpoch
}
type HistoricalSourceInputSummary struct {
	Protocol, RecipeSHA256, EpochSHA256, ResultSHA256                                                                     string
	SQLIdentitySHA256, MongoIdentitySHA256                                                                                string
	Sources                                                                                                               [4]SourceCopyReceipt
	Pages                                                                                                                 uint64
	CompleteInput, TwoIndependentInputsMatched                                                                            bool
	SQLResponsibilityRequired, MongoResponsibilityRequired, AIResponsibilityRequired, FreshComponentQualificationRequired bool
	BusinessClosureVerified, CASAuthorized, DropReady                                                                     bool
	CaptureStopped, LiveSQLGraphRetained                                                                                  bool
}

func (*HistoricalSourceInputRecipe) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputRecipe) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputEpoch) MarshalJSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputEpoch) MarshalBSON() ([]byte, error)  { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputPair) MarshalJSON() ([]byte, error)   { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputPair) MarshalBSON() ([]byte, error)   { return nil, ErrSourceSerialization }
func (*HistoricalSourceInputRecipe) String() string {
	return "private frozen source input recipe; no write authority"
}
func (*HistoricalSourceInputEpoch) String() string {
	return "private native source input epoch; no write authority"
}
func (*HistoricalSourceInputPair) String() string {
	return "private two source inputs; fresh responsibility qualification required"
}

func FreezeHistoricalSourceInputRecipe(ctx context.Context, binding *OriginCopyBinding) (*HistoricalSourceInputRecipe, error) {
	if binding.alive(ctx) != nil || binding.hash == "" {
		return nil, ErrSourceOrigin
	}
	r := &HistoricalSourceInputRecipe{binding: *binding, hash: binding.hash}
	r.self = r
	return r, nil
}
func (r *HistoricalSourceInputRecipe) valid() bool {
	if r == nil || r.self != r || r.hash == "" || r.hash != r.binding.hash || r.binding.copies == nil || !r.binding.copies.complete || !r.binding.limits.valid() {
		return false
	}
	h, e := sourceOriginDigest(struct {
		Expected [4]SourceCopyExpectation
		Hashes   [4]string
		Bytes    [4]uint64
	}{r.binding.expected, r.binding.fileHashes, r.binding.fileBytes})
	return e == nil && h == r.hash
}

// alive checks only actual borrowed scope identity and the separate absolute
// input deadline. Native SQL metadata is checked at object boundaries; there
// is no per-row SQL query. No previous capability's expiry is renewed.
func (e *HistoricalSourceInputEpoch) alive(ctx context.Context) error {
	if e == nil || e.self != e || e.poisoned || e.captureStopped || e.sql == nil || !e.recipe.valid() || ctx == nil || ctx.Err() != nil || !time.Now().Before(e.started.Add(e.budget)) {
		return ErrSourceOrigin
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool != e.sqlConnection {
		return ErrSourceOriginFresh
	}
	s, id, at, err := snapshotInputNative(ctx, true)
	if err != nil || s != e.mongoSession || !bytes.Equal(id, e.mongoSessionID) || at != e.mongoTime || !time.Now().Before(e.mongo.started.Add(e.mongo.limits.MaxDuration)) {
		return ErrMongoSnapshotInput
	}
	return nil
}
func (e *HistoricalSourceInputEpoch) validFile(ctx context.Context) error {
	if e == nil || e.self != e || e.poisoned || e.file == nil || ctx == nil || ctx.Err() != nil || !e.recipe.valid() {
		return ErrSourceOrigin
	}
	info, err := e.file.Stat()
	if err != nil {
		return ErrSourceOrigin
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || uint64(st.Dev) != e.dev || uint64(st.Ino) != e.ino || info.Size() != e.end {
		return ErrSourceOrigin
	}
	return nil
}
func (e *HistoricalSourceInputEpoch) freeze(ctx context.Context, f sourceOriginInputFrame) error {
	if e.alive(ctx) != nil || e.validFile(ctx) != nil || f.Source < 0 || f.Source > 3 || f.Boundary != e.recipe.binding.expected[f.Source].Boundary {
		return ErrSourceOrigin
	}
	f.Version, f.Recipe, f.Epoch = 1, e.recipe.hash, e.epochHash
	disk := sourceOriginInputDiskFrame{Version: f.Version, Source: f.Source, Recipe: f.Recipe, Epoch: f.Epoch, Boundary: f.Boundary, MongoRows: f.MongoRows, EOF: f.EOF}
	var err error
	if f.Source != 3 {
		disk.ColumnsJSON, err = json.Marshal(f.Columns)
		if err != nil {
			return ErrSourceSchema
		}
		encoded := make(SQLColumns, len(f.SQLRows))
		for i, row := range f.SQLRows {
			encoded[i] = make([]*string, len(row))
			for j, cell := range row {
				if cell != nil {
					v := base64.StdEncoding.EncodeToString([]byte(*cell))
					encoded[i][j] = &v
				}
			}
		}
		disk.SQLRowsJSON, err = json.Marshal(encoded)
		if err != nil {
			return ErrSourceSchema
		}
	}
	raw, err := historicalSpoolEncode(disk)
	// Bounded one-page allocation, never a full copy held in memory. SQL NULL
	// cells stay distinct and exact Mongo BSON is never reserialized.
	var maxBytes uint64
	for _, n := range e.recipe.binding.fileBytes {
		maxBytes += n
	}
	maxBytes = maxBytes*2 + (64 << 20)
	if err != nil || len(raw) > 2*sourceOriginInputPageBytes+(1<<20) || uint64(e.end)+uint64(len(raw)) > maxBytes || len(e.pages) > int(e.recipe.binding.limits.MaxRows)+4 {
		return ErrSourceBounds
	}
	ref := historicalSpoolRef{Offset: e.end, Length: int64(len(raw)), SHA256: historicalSpoolSHA(raw)}
	n, err := e.file.WriteAt(raw, e.end)
	if err != nil || n != len(raw) {
		e.poisoned = true
		return ErrSourceOrigin
	}
	e.end += int64(n)
	if e.file.Sync() != nil || e.validFile(ctx) != nil || e.alive(ctx) != nil {
		e.poisoned = true
		return ErrSourceOrigin
	}
	e.pages = append(e.pages, ref)
	return nil
}

// PrepareHistoricalSourceInputEpoch requires real complete SQL input and the
// actual native nontransaction Mongo snapshot. It does not create/close either
// scope. Unsupported snapshots, history eviction, drift and budget exhaustion
// fail; the host must not replace the selected snapshot and retry this epoch.
func PrepareHistoricalSourceInputEpoch(parent context.Context, r *HistoricalSourceInputRecipe, sql *SQLResponsibilitySnapshot, mgo *MongoSnapshotInputEpoch, file *os.File, budget time.Duration) (result *HistoricalSourceInputEpoch, err error) {
	if parent == nil || !r.valid() || r.captureStopped || sql == nil || sql.cycle == nil || sql.Report().CompletedAt.IsZero() || mgo == nil || !mgo.complete || file == nil || budget <= 0 || budget > 30*time.Minute {
		return nil, ErrSourceOrigin
	}
	if sql.ValidateBorrowedSnapshot(parent) != nil || mgo.ValidateBorrowedInputEpoch(parent) != nil {
		return nil, ErrSourceOrigin
	}
	tx, err := hostmysql.RequireTx(parent)
	if err != nil || tx.Statement == nil {
		return nil, ErrSourceOrigin
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ErrSourceOrigin
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || info.Size() != 0 {
		return nil, ErrSourceOrigin
	}
	e := &HistoricalSourceInputEpoch{recipe: r, sql: sql.cycle, mongo: mgo, sqlConnection: tx.Statement.ConnPool, sqlCycleID: sql.Report().CycleID, sqlIdentity: sql.Report().DatabaseIdentitySHA256, mongoIdentity: mgo.metadata.identity, mongoMetadata: mgo.metadata.hash, mongoSession: mgo.session, mongoSessionID: append(bson.Raw(nil), mgo.sessionID...), mongoTime: mgo.snapshot, started: time.Now(), budget: budget, file: file, dev: uint64(st.Dev), ino: uint64(st.Ino)}
	e.self = e
	defer func() {
		if err != nil {
			e.poisoned = true
		}
	}()
	scope, cancel := context.WithDeadline(parent, e.started.Add(budget))
	defer cancel()
	ctx := mongo.NewSessionContext(scope, mgo.session)
	e.sqlHead, err = sourceOriginSQLHead(ctx, tx, r.binding.limits)
	if err != nil {
		return nil, err
	}
	e.epochHash, err = sourceOriginDigest(struct {
		Recipe, SQLCycle, SQLIdentity, SQLHead, MongoIdentity, MongoMetadata string
		Session                                                              bson.Raw
		Time                                                                 primitive.Timestamp
	}{r.hash, e.sqlCycleID, e.sqlIdentity, e.sqlHead, e.mongoIdentity, e.mongoMetadata, e.mongoSessionID, e.mongoTime})
	if err != nil {
		return nil, err
	}
	for i := 0; i < 3; i++ {
		if e.sql.ValidateBorrowedSnapshot(ctx) != nil {
			return nil, ErrSourceOrigin
		}
		e.boundaries[i], e.receipts[i], err = sourceOriginReadSQLWithInput(ctx, tx, &r.binding, i, e.alive, e.freeze)
		if err != nil {
			return nil, err
		}
	}
	// This private read adapter never becomes complete or escapes as an old
	// Mongo global capability. Only the existing source decoder/pager is reused.
	adapter := &MongoResponsibilitySnapshot{db: mgo.db, metadata: mgo.metadata}
	e.boundaries[3], e.receipts[3], err = sourceOriginReadMongoWithInput(ctx, adapter, &r.binding, e.alive, e.freeze)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 4; i++ {
		if e.boundaries[i] != r.binding.expected[i].Boundary || e.receipts[i] != r.binding.copies.receipts[i] {
			return nil, ErrSourceOrigin
		}
	}
	if e.alive(ctx) != nil || e.sql.ValidateBorrowedSnapshot(ctx) != nil || mgo.ValidateBorrowedInputEpoch(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	endHead, err := sourceOriginSQLHead(ctx, tx, r.binding.limits)
	if err != nil || endHead != e.sqlHead {
		return nil, ErrSourceOrigin
	}
	for i := 0; i < 3; i++ {
		b, _, _, check := sourceOriginSQLMetadata(ctx, tx, r.binding.expected[i], r.binding.limits)
		if check != nil || b != e.boundaries[i] {
			return nil, ErrSourceOrigin
		}
	}
	metadata, err := observeMongoCycleMetadata(ctx, mgo.db, mgo.config)
	if err != nil || metadata.hash != e.mongoMetadata {
		return nil, ErrSourceOrigin
	}
	e.resultHash, err = sourceOriginDigest(struct {
		Recipe, SQLIdentity, SQLHead, MongoIdentity, MongoMetadata string
		Boundaries                                                 [4]SourceBoundary
		Receipts                                                   [4]SourceCopyReceipt
	}{r.hash, e.sqlIdentity, e.sqlHead, e.mongoIdentity, e.mongoMetadata, e.boundaries, e.receipts})
	if err != nil {
		return nil, err
	}
	if e.alive(ctx) != nil || e.validFile(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	e.complete = true
	return e, nil
}

func (e *HistoricalSourceInputEpoch) verifyFrozen(ctx context.Context) error {
	if e.validFile(ctx) != nil || !e.complete || e.epochHash == "" || e.resultHash == "" {
		return ErrSourceOrigin
	}
	var ended [4]bool
	last := 0
	for _, ref := range e.pages {
		if ctx.Err() != nil {
			return ErrSourceOrigin
		}
		if ref.Offset < 0 || ref.Length <= 0 || ref.Length > 2*sourceOriginInputPageBytes+(1<<20) || ref.Offset > e.end-ref.Length {
			return ErrSourceOrigin
		}
		raw := make([]byte, int(ref.Length))
		n, err := e.file.ReadAt(raw, ref.Offset)
		if err != nil || n != len(raw) || historicalSpoolSHA(raw) != ref.SHA256 {
			return ErrSourceOrigin
		}
		var f sourceOriginInputDiskFrame
		if historicalSpoolDecode(raw, &f) != nil || f.Version != 1 || f.Source < 0 || f.Source > 3 || f.Source < last || f.Source > last+1 || f.Recipe != e.recipe.hash || f.Epoch != e.epochHash || f.Boundary != e.boundaries[f.Source] || ended[f.Source] || f.Source != 3 && len(f.MongoRows) != 0 || f.Source == 3 && (len(f.ColumnsJSON) != 0 || len(f.SQLRowsJSON) != 0) {
			return ErrSourceOrigin
		}
		if f.Source > last && !ended[last] {
			return ErrSourceOrigin
		}
		last = f.Source
		ended[f.Source] = f.EOF
	}
	for _, yes := range ended {
		if !yes {
			return ErrSourceOrigin
		}
	}
	return e.validFile(ctx)
}
func (e *HistoricalSourceInputEpoch) Summary() HistoricalSourceInputSummary {
	r := HistoricalSourceInputSummary{Protocol: "historical-four-source-input/v1", SQLResponsibilityRequired: true, MongoResponsibilityRequired: true, AIResponsibilityRequired: true, FreshComponentQualificationRequired: true}
	if e != nil && e.self == e && e.complete && !e.poisoned {
		r.RecipeSHA256 = e.recipe.hash
		r.EpochSHA256 = e.epochHash
		r.ResultSHA256 = e.resultHash
		r.SQLIdentitySHA256 = e.sqlIdentity
		r.MongoIdentitySHA256 = e.mongoIdentity
		r.Sources = e.receipts
		r.Pages = uint64(len(e.pages))
		r.CompleteInput = true
		r.CaptureStopped = e.captureStopped
		r.LiveSQLGraphRetained = e.sql != nil
	}
	return r
}

// StopCapture consumes only this input read scope. Its original SQL8 graph
// becomes unreachable from both source and AI inputs; verified disk pages and
// native identities remain. The host still owns rollback/session/file close.
// It cannot keep a transaction, previous capability or write permit alive.
func (e *HistoricalSourceInputEpoch) StopCapture(ctx context.Context) error {
	if e == nil || e.captureStopped || e.alive(ctx) != nil || !e.complete || e.verifyFrozen(ctx) != nil || e.sql.ValidateBorrowedSnapshot(ctx) != nil || e.mongo.ValidateBorrowedInputEpoch(ctx) != nil {
		return ErrSourceOrigin
	}
	e.captureStopped = true
	e.sql = nil
	return nil
}

// ReleaseCaptureIndex retains the already authenticated receipts and private
// file hashes, after both scopes were stopped. No new source membership can
// be captured from this recipe, and no existing old capability is modified.
func (p *HistoricalSourceInputPair) ReleaseCaptureIndex(ctx context.Context) error {
	if p.ValidateFrozen(ctx) != nil || !p.first.captureStopped || !p.second.captureStopped {
		return ErrSourceOrigin
	}
	for _, e := range []*HistoricalSourceInputEpoch{p.first, p.second} {
		r := e.recipe
		if !r.captureStopped {
			compact := *r.binding.copies
			compact.rows, compact.eventIDs, compact.pairs = nil, nil, nil
			compact.reservation = 0
			r.binding.copies = &compact
			r.captureStopped = true
		}
	}
	return p.ValidateFrozen(ctx)
}

// CompareIndependentHistoricalSourceInputs validates every original spool
// frame after the scopes have ended. Matching pure input is deliberately not
// a current responsibility, source write permit, host commit or DROP proof.
func CompareIndependentHistoricalSourceInputs(ctx context.Context, first, second *HistoricalSourceInputEpoch) (*HistoricalSourceInputPair, error) {
	if first == nil || second == nil || first == second || first.verifyFrozen(ctx) != nil || second.verifyFrozen(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	// A second real read may select the same majority snapshot when no Mongo
	// write occurred. Actual session/SQL transaction identities must differ;
	// only a strictly older server-selected time is rejected.
	if first.sqlConnection == second.sqlConnection || first.sqlCycleID == second.sqlCycleID || first.mongoSession == second.mongoSession || (second.mongoTime.T < first.mongoTime.T || second.mongoTime.T == first.mongoTime.T && second.mongoTime.I < first.mongoTime.I) {
		return nil, ErrSourceOriginFresh
	}
	if first.recipe.hash != second.recipe.hash || first.resultHash != second.resultHash || first.receipts != second.receipts || first.boundaries != second.boundaries {
		return nil, ErrSourceOrigin
	}
	pair := &HistoricalSourceInputPair{first: first, second: second}
	pair.self = pair
	return pair, nil
}
func (p *HistoricalSourceInputPair) Summary() HistoricalSourceInputSummary {
	if p == nil || p.self != p || p.first == nil || p.second == nil {
		return (*HistoricalSourceInputEpoch)(nil).Summary()
	}
	r := p.second.Summary()
	r.TwoIndependentInputsMatched = r.CompleteInput
	return r
}

// ValidateFrozen checks unchanged raw input after read scopes end. It is not
// a liveness, source write, current responsibility or transaction validator.
func (p *HistoricalSourceInputPair) ValidateFrozen(ctx context.Context) error {
	if p == nil || p.self != p {
		return ErrSourceOrigin
	}
	_, err := CompareIndependentHistoricalSourceInputs(ctx, p.first, p.second)
	return err
}

// This is a current component READ, not a source-writer fence or a qualified
// CAS plan. SQL has no indexed aggregate/owner negative selector in the retired
// schema; matching known PKs must never be advertised as excluding new sources.
// The host supplies the same fresh native SQL and Mongo transactions used by
// the business observers. This object neither starts nor ends either scope.
type HistoricalComponentSourceObservation struct {
	self          *HistoricalComponentSourceObservation
	component     *HistoricalCASComponent
	pair          *HistoricalSourceInputPair
	mongo         *MongoHistoricalComponentObservation
	sql           []*sqlevaluation.SQLHistoricalComponentObservation
	started       time.Time
	budget        time.Duration
	seal, rowsSHA string
	rows, bytes   uint64
	mongoNegative bool
}

type HistoricalComponentSourceSummary struct {
	Protocol, ComponentSHA256, SourceInputSHA256, SourceRowsSHA256                                                                                         string
	Rows, Bytes                                                                                                                                            uint64
	ActualNativeScope, SelectedSourceBytesMatched, MongoAggregateNegativeRangesMatched                                                                     bool
	SQLSourceNegativeClosureRequired, MongoOwnerSourceNegativeClosureRequired, SourceWriterFenceRequired, CurrentMessageClosureRequired, AIClosureRequired bool
	SourceClosureVerified, CASAuthorized, DropReady                                                                                                        bool
}

func (*HistoricalComponentSourceObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalComponentSourceObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalComponentSourceObservation) String() string {
	return "private fresh source rows; SQL source negative closure and writer fence unproven"
}
func (o *HistoricalComponentSourceObservation) GoString() string { return o.String() }

func sourceComponentPairIntact(ctx context.Context, p *HistoricalSourceInputPair) error {
	if p == nil || p.self != p || p.first == nil || p.second == nil || p.first == p.second || p.first.validFile(ctx) != nil || p.second.validFile(ctx) != nil || !p.first.complete || !p.second.complete || p.first.recipe.hash != p.second.recipe.hash || p.first.resultHash == "" || p.first.resultHash != p.second.resultHash || p.first.receipts != p.second.receipts || p.first.boundaries != p.second.boundaries || p.first.sqlConnection == p.second.sqlConnection || p.first.sqlCycleID == p.second.sqlCycleID || p.first.mongoSession == p.second.mongoSession || p.second.mongoTime.T < p.first.mongoTime.T || p.second.mongoTime.T == p.first.mongoTime.T && p.second.mongoTime.I < p.first.mongoTime.I {
		return ErrSourceOrigin
	}
	return nil
}

func (o *HistoricalComponentSourceObservation) ValidateBorrowedObservation(ctx context.Context) error {
	if o == nil || o.self != o || o.component == nil || o.pair == nil || ctx == nil || ctx.Err() != nil || o.rowsSHA == "" || !time.Now().Before(o.started.Add(o.budget)) || o.seal != mongoHistoricalComponentInputSeal(o.component) || sourceComponentPairIntact(ctx, o.pair) != nil || o.mongo == nil || !o.mongo.complete || o.mongo.component != o.component || o.mongo.validate(ctx) != nil || len(o.sql) != len(o.component.inputs) {
		return ErrSourceOrigin
	}
	for i, sql := range o.sql {
		if sql == nil || sql.ValidateBorrowedObservation(ctx) != nil {
			return ErrSourceOriginFresh
		}
		view, err := sql.SemanticView(ctx)
		if err != nil || view.MatchesInput(ctx, o.component.inputs[i].sqlRecipe) != nil {
			return ErrSourceOriginFresh
		}
	}
	return nil
}

func (o *HistoricalComponentSourceObservation) Summary() HistoricalComponentSourceSummary {
	r := HistoricalComponentSourceSummary{Protocol: "historical-component-source-read/v1", SQLSourceNegativeClosureRequired: true, MongoOwnerSourceNegativeClosureRequired: true, SourceWriterFenceRequired: true, CurrentMessageClosureRequired: true, AIClosureRequired: true}
	if o == nil || o.self != o || o.rowsSHA == "" || o.component == nil || o.seal != mongoHistoricalComponentInputSeal(o.component) || o.pair == nil || o.pair.self != o.pair {
		return r
	}
	r.ComponentSHA256, r.SourceInputSHA256, r.SourceRowsSHA256 = o.seal, o.pair.second.resultHash, o.rowsSHA
	r.Rows, r.Bytes = o.rows, o.bytes
	r.ActualNativeScope, r.SelectedSourceBytesMatched, r.MongoAggregateNegativeRangesMatched = true, true, o.mongoNegative
	return r
}

// Read only the original selected frame, checking its immutable write-time
// hash and decoded facts. This does not BindEvent or re-create the coordinator,
// coverage, source authentication capability or expired origin qualification.
func sourceComponentFrozenEvent(ctx context.Context, index *WholeSourceJointIndex, pair *HistoricalSourceInputPair, id string) (*DecodedSourceEvent, error) {
	if ctx == nil || ctx.Err() != nil || index == nil || !index.complete || pair == nil || pair.self != pair || index.encodedSHA != pair.second.recipe.binding.fileHashes || index.receipts != pair.second.receipts {
		return nil, ErrSourceAuthentication
	}
	entry, ok := index.entries[id]
	if !ok || entry.EventID != id || entry.Key.object != 0 && entry.Key.object != 3 || entry.Length <= 0 || entry.Length > 2*MaxSourceRowBytes+8 || entry.Offset < 0 {
		return nil, ErrSourceAuthentication
	}
	copy := index.copies[entry.Key.object]
	if copy.Expected != pair.second.recipe.binding.expected[entry.Key.object] {
		return nil, ErrSourceAuthentication
	}
	file, ok := copy.Input.(*os.File)
	if !ok || file == nil {
		return nil, ErrSourceOrigin
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() < entry.Length || entry.Offset > info.Size()-entry.Length {
		return nil, ErrSourceOrigin
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) {
		return nil, ErrSourceOrigin
	}
	raw := make([]byte, int(entry.Length))
	if n, err := file.ReadAt(raw, entry.Offset); err != nil || n != len(raw) || sourceSHA(raw) != entry.PhysicalSHA256 {
		return nil, ErrSourceAuthentication
	}
	var facts *DecodedSourceEvent
	if entry.Key.object == 0 {
		reader, err := NewSQLSourceReader(io.MultiReader(bytes.NewReader(index.headers[0]), bytes.NewReader(raw)), copy.Expected)
		if err != nil {
			return nil, err
		}
		facts, err = reader.Next()
		if err != nil {
			return nil, err
		}
	} else {
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), copy.Expected)
		if err != nil {
			return nil, err
		}
		facts, err = reader.Next()
		if err != nil {
			return nil, err
		}
	}
	sha, err := privateFactsSHA(facts)
	key, keyErr := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
	if err != nil || keyErr != nil || sha != entry.FactsSHA256 || key != entry.Key || facts.EventID != id || facts.OrgID != entry.OrgID || facts.EventType != entry.EventType || facts.AggregateType != entry.AggregateType || facts.AggregateID != entry.AggregateID {
		return nil, ErrSourceAuthentication
	}
	return facts, nil
}

func sourceComponentEvents(ctx context.Context, c *HistoricalCASComponent, p *HistoricalSourceInputPair) (map[string]*DecodedSourceEvent, error) {
	out := map[string]*DecodedSourceEvent{}
	for _, f := range c.inputs {
		ids, err := f.sqlRecipe.SourceEventIDs()
		if err != nil || len(ids) == 0 {
			return nil, ErrSourceAuthentication
		}
		for _, id := range ids {
			facts, err := sourceComponentFrozenEvent(ctx, f.index, p, id)
			if err != nil {
				return nil, err
			}
			if prior, found := out[id]; found {
				first, e1 := privateFactsSHA(prior)
				second, e2 := privateFactsSHA(facts)
				if e1 != nil || e2 != nil || first != second {
					return nil, ErrSourceAuthentication
				}
			} else {
				out[id] = facts
			}
			if len(out) > 4096 {
				return nil, ErrSourceBounds
			}
		}
	}
	return out, nil
}

func sourceComponentSourceMatch(expected, actual *DecodedSourceEvent) error {
	first, e1 := privateFactsSHA(expected)
	second, e2 := privateFactsSHA(actual)
	if e1 != nil || e2 != nil || first != second {
		return ErrSourceAuthentication
	}
	return nil
}

// The Mongo query does not constrain event type, organization, state or the
// old upper bound: newly inserted unsupported/cross-org sources also reject.
// The approved compound index is validated, never invented or replaced by an
// unhinted collection scan. A missing/ineligible index fails this observation.
func sourceComponentMongoIndex(metadata mongoCycleMetadata) error {
	definition, ok := metadata.definitions["domain_event_outbox"]
	if !ok {
		return ErrSourceSchema
	}
	for _, raw := range definition.indexes {
		if raw.Lookup("name").Type != bson.TypeString || raw.Lookup("name").StringValue() != "idx_outbox_consistency_audit" {
			continue
		}
		fields, err := exactBSONFields(raw)
		if err != nil || fields["partialFilterExpression"].Type != 0 || fields["sparse"].Type != 0 && (fields["sparse"].Type != bson.TypeBoolean || fields["sparse"].Boolean()) {
			return ErrSourceSchema
		}
		if c := fields["collation"]; c.Type != 0 && (c.Type != bson.TypeEmbeddedDocument || c.Document().Lookup("locale").Type != bson.TypeString || c.Document().Lookup("locale").StringValue() != "simple") {
			return ErrSourceSchema
		}
		if fields["key"].Type != bson.TypeEmbeddedDocument {
			return ErrSourceSchema
		}
		keys, err := fields["key"].Document().Elements()
		if err != nil || len(keys) != 3 {
			return ErrSourceSchema
		}
		for i, name := range []string{"aggregate_type", "event_type", "aggregate_id"} {
			n, integer := mongoExactInteger(keys[i].Value())
			if keys[i].Key() != name || !integer || n != 1 {
				return ErrSourceSchema
			}
		}
		return nil
	}
	return ErrSourceSchema
}

func sourceComponentReadMongo(ctx context.Context, o *HistoricalComponentSourceObservation, expected map[string]*DecodedSourceEvent, metadata mongoCycleMetadata, limits SourceOriginLimits) (map[string]*DecodedSourceEvent, error) {
	if sourceComponentMongoIndex(metadata) != nil {
		return nil, ErrSourceSchema
	}
	boundary, err := sourceOriginMongoDefinitionMetadata(metadata)
	original := o.pair.second.boundaries[3]
	if err != nil || boundary.SchemaHash != original.SchemaHash || boundary.IdentityHash != original.IdentityHash {
		return nil, ErrSourceOrigin
	}
	groups := map[string]map[string]bool{}
	for _, facts := range expected {
		if groups[facts.AggregateType] == nil {
			groups[facts.AggregateType] = map[string]bool{}
		}
		groups[facts.AggregateType][facts.AggregateID] = true
	}
	actual := map[string]*DecodedSourceEvent{}
	for _, kind := range sourceComponentSortedKeys(groups) {
		var ids bson.A
		for _, id := range sourceComponentSortedKeys(groups[kind]) {
			ids = append(ids, id)
		}
		q, cancel := context.WithTimeout(ctx, limits.QueryTimeout)
		cursor, err := o.mongo.db.Collection("domain_event_outbox").Find(q, bson.D{{Key: "aggregate_type", Value: kind}, {Key: "aggregate_id", Value: bson.D{{Key: "$in", Value: ids}}}}, options.Find().SetHint("idx_outbox_consistency_audit").SetCollation(&options.Collation{Locale: "simple"}).SetLimit(4097).SetBatchSize(128).SetMaxTime(limits.QueryTimeout))
		if err != nil {
			cancel()
			return nil, ErrSourceOrigin
		}
		reader, readErr := NewMongoSourceReader(bytes.NewReader(nil), o.pair.second.recipe.binding.expected[3])
		var count int
		for readErr == nil && cursor.Next(q) {
			if o.mongo.validate(q) != nil || len(cursor.Current) > MaxSourceRowBytes || o.bytes+uint64(len(cursor.Current)) > 64<<20 || count >= 4096 {
				readErr = ErrSourceBounds
				break
			}
			// Compound order is not _id order. Decode each exact original raw
			// independently while still enforcing the fixed BSON type/upper.
			reader.hasLast = false
			facts, err := reader.decodeRaw(cursor.Current)
			if err != nil || facts.Source.Database != "mongodb" || expected[facts.EventID] == nil || expected[facts.EventID].Source.Database != "mongodb" || actual[facts.EventID] != nil || sourceComponentSourceMatch(expected[facts.EventID], facts) != nil {
				readErr = ErrSourceAuthentication
				break
			}
			actual[facts.EventID] = facts
			o.bytes += uint64(len(cursor.Current))
			count++
		}
		cursorErr, closeErr := cursor.Err(), cursor.Close(q)
		cancel()
		if readErr != nil {
			return nil, readErr
		}
		if cursorErr != nil || closeErr != nil {
			return nil, ErrSourceOrigin
		}
	}
	for id, facts := range expected {
		if facts.Source.Database == "mongodb" && actual[id] == nil {
			return nil, ErrSourceAuthentication
		}
	}
	return actual, nil
}

func sourceComponentSQLProjection(columns SQLColumns) string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = "CAST(" + sourceOriginQuote(*c[0]) + " AS BINARY)"
	}
	return strings.Join(out, ",")
}

// A PK read can verify the chosen original bytes, never the absence of newly
// inserted owner-related SQL events. The latter remains an explicit REQUIRED
// typed source-writer/negative-closure input to the separate composition.
func PrepareHistoricalComponentSourceObservation(parent context.Context, c *HistoricalCASComponent, pair *HistoricalSourceInputPair, mgo *MongoHistoricalComponentObservation, sql []*sqlevaluation.SQLHistoricalComponentObservation, budget time.Duration) (*HistoricalComponentSourceObservation, error) {
	seal := mongoHistoricalComponentInputSeal(c)
	if parent == nil || parent.Err() != nil || seal == "" || sourceComponentPairIntact(parent, pair) != nil || mgo == nil || mgo.component != c || !mgo.complete || mgo.validate(parent) != nil || len(sql) != len(c.inputs) || budget <= 0 || budget > 20*time.Second || pair.second.mongo != mgo.input || mgo.input.metadata.identity != pair.second.mongoIdentity || mgo.input.metadata.hash != pair.second.mongoMetadata {
		return nil, ErrSourceOrigin
	}
	o := &HistoricalComponentSourceObservation{component: c, pair: pair, mongo: mgo, sql: append([]*sqlevaluation.SQLHistoricalComponentObservation(nil), sql...), started: time.Now(), budget: budget, seal: seal, rowsSHA: "preparing"}
	o.self = o
	ctx, cancel := context.WithDeadline(parent, o.started.Add(budget))
	defer cancel()
	if o.ValidateBorrowedObservation(ctx) != nil {
		return nil, ErrSourceOriginFresh
	}
	expected, err := sourceComponentEvents(ctx, c, pair)
	if err != nil {
		return nil, err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, ErrSourceOriginFresh
	}
	limits := pair.second.recipe.binding.limits
	identity, err := sourceOriginSQLQuery(ctx, tx, limits, "SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY)")
	if err != nil || len(identity) != 1 || len(identity[0]) != 2 || sourceOriginSQLValue(identity[0], 0) == "" || sourceOriginSQLValue(identity[0], 1) == "" || mongoOwnerHashParts("mysql_database_identity_v1", sourceOriginSQLValue(identity[0], 0), sourceOriginSQLValue(identity[0], 1)) != pair.second.sqlIdentity {
		return nil, ErrSourceOrigin
	}
	head, err := sourceOriginSQLHead(ctx, tx, limits)
	if err != nil || head != pair.second.sqlHead {
		return nil, ErrSourceOrigin
	}
	boundary, columns, _, err := sourceOriginSQLMetadata(ctx, tx, pair.second.recipe.binding.expected[0], limits)
	if err != nil || boundary != pair.second.boundaries[0] {
		return nil, ErrSourceOrigin
	}
	columnJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, ErrSourceSchema
	}
	upper, err := canonicalBase64(boundary.UpperToken)
	if err != nil && !boundary.Empty {
		return nil, ErrSourceSchema
	}
	var upperID uint64
	if !boundary.Empty {
		upperID, err = positiveSQLID(upper)
		if err != nil {
			return nil, err
		}
	}
	parts := []string{"historical-component-current-source/v1", seal, pair.second.resultHash}
	sqlExpected := map[uint64]*DecodedSourceEvent{}
	var sqlIDs []uint64
	for _, id := range sourceComponentSortedKeys(expected) {
		facts := expected[id]
		if facts.Source.Database != "mysql" {
			continue
		}
		pk, err := canonicalBase64(facts.PrimaryKeyToken)
		if err != nil {
			return nil, err
		}
		key, err := positiveSQLID(pk)
		if err != nil || sqlExpected[key] != nil {
			return nil, ErrSourceAuthentication
		}
		sqlExpected[key] = facts
		sqlIDs = append(sqlIDs, key)
	}
	if len(sqlIDs) > 0 {
		// One native primary-key set read, not a transaction/schema query per
		// row. Exact set equality below rejects missing and substituted keys.
		rows, err := sourceOriginSQLQuery(ctx, tx, limits, "SELECT "+sourceComponentSQLProjection(columns)+" FROM `domain_event_outbox` FORCE INDEX (PRIMARY) WHERE id IN ? ORDER BY id LIMIT 4097", sqlIDs)
		if err != nil || len(rows) != len(sqlIDs) {
			return nil, ErrSourceOrigin
		}
		reader := &SQLSourceReader{acc: sourceAccumulator{expectation: pair.second.recipe.binding.expected[0], h: sha256.New()}, columns: columns, columnsHash: sourceSHA(columnJSON), upper: upperID}
		for _, row := range rows {
			encoded := make([]*string, len(row))
			for i, cell := range row {
				if cell != nil {
					value := base64.StdEncoding.EncodeToString([]byte(*cell))
					encoded[i] = &value
					o.bytes += uint64(len(*cell))
				}
			}
			if ctx.Err() != nil || o.bytes > 64<<20 {
				return nil, ErrSourceBounds
			}
			line, err := json.Marshal(encoded)
			if err != nil {
				return nil, ErrSourceSchema
			}
			actual, err := reader.decodeLine(line)
			if err != nil {
				return nil, err
			}
			pk, err := canonicalBase64(actual.PrimaryKeyToken)
			if err != nil {
				return nil, err
			}
			key, err := positiveSQLID(pk)
			if err != nil || sqlExpected[key] == nil || sourceComponentSourceMatch(sqlExpected[key], actual) != nil {
				return nil, ErrSourceAuthentication
			}
			delete(sqlExpected, key)
			o.rows++
		}
		if len(sqlExpected) != 0 || o.ValidateBorrowedObservation(ctx) != nil {
			return nil, ErrSourceAuthentication
		}
	}
	metadata, err := observeMongoCycleMetadata(ctx, mgo.db, c.inputs[0].mongoRead.config)
	if err != nil || metadata.hash != mgo.metadata {
		return nil, ErrSourceOrigin
	}
	mongoRows, err := sourceComponentReadMongo(ctx, o, expected, metadata, limits)
	if err != nil {
		return nil, err
	}
	o.rows += uint64(len(mongoRows))
	for _, id := range sourceComponentSortedKeys(expected) {
		sha, err := privateFactsSHA(expected[id])
		if err != nil {
			return nil, err
		}
		parts = append(parts, id, hex.EncodeToString(sha[:]))
	}
	metadata, err = observeMongoCycleMetadata(ctx, mgo.db, c.inputs[0].mongoRead.config)
	if err != nil || metadata.hash != mgo.metadata {
		return nil, ErrSourceOrigin
	}
	after, _, _, err := sourceOriginSQLMetadata(ctx, tx, pair.second.recipe.binding.expected[0], limits)
	afterHead, headErr := sourceOriginSQLHead(ctx, tx, limits)
	if err != nil || after != boundary || headErr != nil || afterHead != head || o.ValidateBorrowedObservation(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	o.rowsSHA, o.mongoNegative = mongoOwnerHashParts(parts...), true
	return o, nil
}

func sourceComponentSortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
