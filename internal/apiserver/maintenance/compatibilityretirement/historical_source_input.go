package retirement

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"syscall"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
