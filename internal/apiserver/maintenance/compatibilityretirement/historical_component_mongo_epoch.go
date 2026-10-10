package retirement

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const ErrMongoHistoricalComponentEpoch SourceError = "mongo_historical_component_epoch_rejected"

// The recipe freezes actual RANGE selectors as well as row fingerprints.
// Fingerprints alone cannot detect a newly inserted descendant or a formerly
// absent range. This recipe is pure input, not an imported CAS plan.
type mongoHistoricalComponentReadRecipe struct {
	config    MongoOwnerConfig
	metadata  string
	selection mongoBatchSelection
	limits    MongoHistoricalOwnerBatchLimits
	hints     map[string]string
	original  mongoCycleTxn
	// Snapshot-only input has a distinct origin; no transaction is invented.
	snapshotEpoch, snapshotInput, snapshotOwner string
}

func (r *mongoHistoricalComponentReadRecipe) digest() string {
	if r == nil {
		return ""
	}
	parts := []string{"mongo-component-read-recipe/v1", r.config.ExpectedIdentityHash, strconv.FormatInt(r.config.ExpectedMigrationVersion, 10), r.metadata, string(r.original.session), strconv.FormatInt(r.original.number, 10), strconv.Itoa(r.limits.MaxSources), strconv.Itoa(r.limits.MaxRows), strconv.FormatUint(r.limits.MaxBytes, 10), strconv.FormatInt(int64(r.limits.MaxDuration), 10)}
	if r.snapshotEpoch != "" {
		parts = append(parts, "snapshot-input", r.snapshotEpoch, r.snapshotInput, r.snapshotOwner)
	}
	for _, query := range mongoCASSelections(r.selection) {
		parts = append(parts, query.name, query.field, r.hints[query.name+":"+query.field])
		for _, id := range mongoBatchIDs(query.ids) {
			parts = append(parts, strconv.FormatUint(id, 10))
		}
	}
	return mongoOwnerHashParts(parts...)
}

func freezeMongoHistoricalComponentReadRecipe(ctx context.Context, b *MongoHistoricalOwnerBatch) (*mongoHistoricalComponentReadRecipe, error) {
	if b == nil || b.global == nil || b.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	r := &mongoHistoricalComponentReadRecipe{config: b.global.config, metadata: b.global.metadata.hash, selection: mongoCASCloneSelection(b.selection), limits: b.limits, hints: map[string]string{}, original: mongoCycleTxn{append(bson.Raw(nil), b.global.txn.session...), b.global.txn.number}}
	for _, query := range mongoCASSelections(r.selection) {
		if len(query.ids) == 0 {
			continue
		}
		hint, err := b.index(query.name, query.field, query.field == "domain_id")
		if err != nil {
			return nil, err
		}
		r.hints[query.name+":"+query.field] = hint
	}
	if b.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	return r, nil
}

// This factory is only an input bridge for the component planner. The actual
// nontransaction snapshot must still be alive, and selected bytes must match
// its original private frames. It grants no global, transaction or CAS permit.
func freezeMongoSnapshotOwnerComponentReadRecipe(parent context.Context, input *MongoSnapshotInputEpoch, footprint *MongoSnapshotOwnerFootprint) (*mongoHistoricalComponentReadRecipe, error) {
	if input == nil || footprint == nil || input.ValidateBorrowedInputEpoch(parent) != nil || !input.complete || footprint.InputSHA256() == "" {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	deadline := time.Now().Add(footprint.limits.MaxDuration)
	if input.started.Add(input.limits.MaxDuration).Before(deadline) {
		deadline = input.started.Add(input.limits.MaxDuration)
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	summary := input.Summary()
	if footprint.identitySHA != summary.IdentitySHA256 || footprint.metadataSHA != summary.MetadataSHA256 || footprint.nativeSHA != summary.NativeEpochSHA256 || footprint.snapshotSHA != summary.SnapshotSHA256 || input.matchOwnerRows(ctx, footprint.data) != nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	r := &mongoHistoricalComponentReadRecipe{config: input.config, metadata: input.metadata.hash, selection: mongoCASCloneSelection(footprint.selection), limits: footprint.limits, hints: map[string]string{}, snapshotEpoch: summary.NativeEpochSHA256, snapshotInput: summary.SnapshotSHA256, snapshotOwner: footprint.InputSHA256()}
	reader := &mongoOwnerReadCore{metadata: input.metadata}
	for _, query := range mongoCASSelections(r.selection) {
		if len(query.ids) == 0 {
			continue
		}
		hint, err := reader.index(query.name, query.field, query.field == "domain_id")
		if err != nil {
			return nil, err
		}
		r.hints[query.name+":"+query.field] = hint
	}
	metadata, err := observeMongoCycleMetadata(ctx, input.db, input.config)
	if err != nil || metadata.hash != input.metadata.hash || input.ValidateBorrowedInputEpoch(ctx) != nil || footprint.InputSHA256() != r.snapshotOwner {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	return r, nil
}

// Snapshot recipes can only enter a new physical READ, and must bind the same
// real frozen epoch. Old transaction recipes retain their original tuple guard.
func (r *mongoHistoricalComponentReadRecipe) matchesOriginalInput(input *MongoSnapshotInputEpoch, txn mongoCycleTxn) bool {
	if r.snapshotEpoch == "" {
		return r.snapshotInput == "" && r.snapshotOwner == "" && !(txn.number == r.original.number && bytes.Equal(txn.session, r.original.session))
	}
	if input == nil || len(r.original.session) != 0 || r.original.number != 0 || !evidence.ValidSHA256(r.snapshotEpoch) || !evidence.ValidSHA256(r.snapshotInput) || !evidence.ValidSHA256(r.snapshotOwner) {
		return false
	}
	summary := input.Summary()
	return summary.CompleteInput && r.snapshotEpoch == summary.NativeEpochSHA256 && r.snapshotInput == summary.SnapshotSHA256 && r.metadata == summary.MetadataSHA256
}

// Only an actual host snapshot transaction produces this physical observation.
// It does not implement any old global/origin/owner/CAS capability. The host
// opens, commits/aborts and closes its own transaction/session/client.
type MongoHistoricalComponentObservation struct {
	self      *MongoHistoricalComponentObservation
	component *HistoricalCASComponent
	input     *MongoSnapshotInputEpoch
	db        *mongo.Database
	session   mongo.Session
	txn       mongoCycleTxn
	started   time.Time
	budget    time.Duration
	seal      string
	rowsSeal  string
	complete  bool
	applyMu   sync.Mutex
	applied   bool
	metadata  string
	frames    []map[string][]bson.Raw
	rows      uint64
	bytes     uint64
}

type MongoHistoricalComponentObservationSummary struct {
	Protocol, MetadataSHA256, PhysicalRowsSHA256, NativeTransactionSHA256 string
	Frames, Rows, Bytes                                                   uint64
	ActualTransactionRead, FrozenBaselinesMatched                         bool
	FullSourcesRequired, SQLQualificationRequired, AIClosureRequired      bool
	CASAuthorized, HostCommitVerified, DropReady                          bool
}

func (*MongoHistoricalComponentObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalComponentObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalComponentObservation) String() string {
	return "private scoped Mongo physical observation; source/SQL/AI/CAS unqualified"
}
func (o *MongoHistoricalComponentObservation) GoString() string { return o.String() }

func mongoHistoricalComponentInputSeal(c *HistoricalCASComponent) string {
	if c == nil || len(c.inputs) == 0 || len(c.inputs) > 512 {
		return ""
	}
	parts := []string{"mongo-component-input-selection/v1"}
	for _, f := range c.inputs {
		if f == nil || f.self != f || f.seal == "" || f.seal != f.digest() || f.mongoRead == nil || !f.mongoRead.limits.valid() || f.mongoRead.snapshotEpoch != "" && f.mongoCAS != nil {
			return ""
		}
		parts = append(parts, f.seal)
	}
	return mongoOwnerHashParts(parts...)
}

func (o *MongoHistoricalComponentObservation) validate(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || o == nil || o.self != o || o.db == nil || o.input == nil || o.session == nil || !time.Now().Before(o.started.Add(o.budget)) || o.seal == "" || o.seal != mongoHistoricalComponentInputSeal(o.component) || o.input.validFile(ctx) != nil || o.complete && (o.rowsSeal == "" || o.rowsSeal != o.rowsSHA()) {
		return ErrMongoHistoricalComponentEpoch
	}
	txn, err := mongoCycleTransaction(ctx, o.db)
	if err != nil || txn.number != o.txn.number || !bytes.Equal(txn.session, o.txn.session) || mongo.SessionFromContext(ctx) != o.session {
		return ErrMongoHistoricalComponentEpoch
	}
	return nil
}

// Prepare performs actual bounded range reads, including empty ranges, in a
// NEW host transaction. The frozen input cannot supply transaction authority.
// Any changed baseline, added/deleted row, schema, native context or deadline
// rejects this component without replacing the transaction or retrying it.
func PrepareMongoHistoricalComponentObservation(parent context.Context, db *mongo.Database, c *HistoricalCASComponent, input *MongoSnapshotInputEpoch, budget time.Duration) (*MongoHistoricalComponentObservation, error) {
	seal := mongoHistoricalComponentInputSeal(c)
	if parent == nil || parent.Err() != nil || db == nil || input == nil || !input.complete || input.validFile(parent) != nil || db != input.db || seal == "" || budget <= 0 || budget > 20*time.Second {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	txn, err := mongoCycleTransaction(parent, db)
	// The driver's server-session pool legitimately reuses a UUID after an
	// ended session, with an incremented transaction number. Native context,
	// session instance and the full transaction tuple distinguish the epoch;
	// a UUID alone cannot classify it as the old input-only session.
	if err != nil || mongo.SessionFromContext(parent) == input.session {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	o := &MongoHistoricalComponentObservation{component: c, input: input, db: db, session: mongo.SessionFromContext(parent), txn: txn, started: time.Now(), budget: budget, seal: seal, metadata: input.metadata.hash}
	o.self = o
	scope, cancel := context.WithDeadline(parent, o.started.Add(budget))
	defer cancel()
	ctx := mongo.NewSessionContext(scope, o.session)
	metadata, err := observeMongoCycleMetadata(ctx, db, c.inputs[0].mongoRead.config)
	if err != nil || metadata.hash != o.metadata {
		return nil, fmt.Errorf("%w: metadata_before", ErrMongoHistoricalComponentEpoch)
	}
	for _, frame := range c.inputs {
		r := frame.mongoRead
		if r.metadata != o.metadata || r.config != c.inputs[0].mongoRead.config || r.config.ExpectedIdentityHash != input.metadata.identity || !r.matchesOriginalInput(input, txn) || o.validate(ctx) != nil {
			return nil, fmt.Errorf("%w: range_guard", ErrMongoHistoricalComponentEpoch)
		}
		// Only capture is reused. This local read helper has no groups and is
		// never returned, so it cannot be accepted by Apply as a CAS plan.
		reader := &MongoHistoricalBatchCASPlan{db: db, selection: r.selection, limits: r.limits, hints: r.hints}
		actual, err := reader.capture(ctx)
		if err != nil || mongoHistoricalComponentRowsMatch(frame, actual) != nil {
			return nil, ErrMongoBatchConflict
		}
		for _, raws := range actual {
			for _, raw := range raws {
				o.rows++
				o.bytes += uint64(len(raw))
			}
		}
		if o.rows > 131072 || o.bytes > 256<<20 {
			return nil, ErrMongoBatchBounds
		}
		o.frames = append(o.frames, actual)
	}
	metadata, err = observeMongoCycleMetadata(ctx, db, c.inputs[0].mongoRead.config)
	if err != nil || metadata.hash != o.metadata || o.validate(ctx) != nil {
		return nil, fmt.Errorf("%w: metadata_after", ErrMongoHistoricalComponentEpoch)
	}
	o.rowsSeal, o.complete = o.rowsSHA(), true
	if o.validate(ctx) != nil {
		return nil, fmt.Errorf("%w: completed_guard", ErrMongoHistoricalComponentEpoch)
	}
	return o, nil
}

func mongoHistoricalComponentRowsMatch(f *HistoricalCASComponentInput, actual map[string][]bson.Raw) error {
	expected := map[historicalCASRowKey]string{}
	for _, row := range f.rows {
		if row.key.store == "mongodb" {
			expected[row.key] = row.sha
		}
	}
	for name, raws := range actual {
		for _, raw := range raws {
			id, err := historicalSpoolMongoID(raw)
			key := historicalCASRowKey{"mongodb", name, id}
			if err != nil || expected[key] != historicalSpoolSHA(raw) {
				return ErrMongoBatchConflict
			}
			delete(expected, key)
		}
	}
	if len(expected) != 0 {
		return ErrMongoBatchConflict
	}
	return nil
}

func (o *MongoHistoricalComponentObservation) rowsSHA() string {
	if o == nil {
		return ""
	}
	parts := []string{"mongo-component-physical-rows/v1", o.metadata, o.seal}
	for _, frame := range o.frames {
		parts = append(parts, mongoCASHash(frame, o.metadata))
	}
	return mongoOwnerHashParts(parts...)
}

func (o *MongoHistoricalComponentObservation) Summary() MongoHistoricalComponentObservationSummary {
	r := MongoHistoricalComponentObservationSummary{Protocol: "mongo-component-physical-observation/v1", FullSourcesRequired: true, SQLQualificationRequired: true, AIClosureRequired: true}
	if o == nil || o.self != o || !o.complete || o.seal == "" || o.seal != mongoHistoricalComponentInputSeal(o.component) || o.rowsSeal == "" || o.rowsSeal != o.rowsSHA() || len(o.frames) != len(o.component.inputs) {
		return r
	}
	r.MetadataSHA256, r.PhysicalRowsSHA256 = o.metadata, o.rowsSHA()
	r.NativeTransactionSHA256 = mongoOwnerHashParts(string(o.txn.session), strconv.FormatInt(o.txn.number, 10))
	r.Frames, r.Rows, r.Bytes = uint64(len(o.frames)), o.rows, o.bytes
	r.ActualTransactionRead, r.FrozenBaselinesMatched = true, true
	return r
}

// This is a genuinely new server read of unchanged physical inputs, not a
// post-CAS completion receipt. Changed evidence requires the actual committed
// statement's expected image and the separate SQL/source/AI protocol.
func (o *MongoHistoricalComponentObservation) VerifyIndependentRead(ctx context.Context, fresh *MongoHistoricalComponentObservation) error {
	if o == nil || o.self != o || !o.complete || o.seal != mongoHistoricalComponentInputSeal(o.component) || o.rowsSeal == "" || o.rowsSeal != o.rowsSHA() || fresh == nil || fresh.self != fresh || !fresh.complete || o == fresh || o.session == fresh.session || fresh.validate(ctx) != nil || o.component != fresh.component || o.input != fresh.input || o.seal != fresh.seal || o.metadata != fresh.metadata || o.txn.number == fresh.txn.number && bytes.Equal(o.txn.session, fresh.txn.session) || fresh.started.Before(o.started) || !reflect.DeepEqual(o.frames, fresh.frames) {
		return ErrMongoHistoricalComponentEpoch
	}
	return nil
}
