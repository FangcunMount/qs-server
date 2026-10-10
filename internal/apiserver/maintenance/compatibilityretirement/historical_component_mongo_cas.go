package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// A recipe retains only the immutable inputs of an originally qualified plan.
// It retains no expiry, database, transaction/session or usable write plan.
// Fresh physical matching alone is not whole-source/SQL/AI authorization.
type mongoHistoricalComponentCASRecipe struct {
	identity, sqlIdentity, sqlRows string
	groups                         []mongoCASGroup
	attachments                    []mongoCASAttachmentFacts
}

func (r *mongoHistoricalComponentCASRecipe) digest() string {
	if r == nil {
		return ""
	}
	parts := []string{"mongo-component-cas-recipe/v1", r.identity, r.sqlIdentity, r.sqlRows}
	for _, group := range r.groups {
		encoded, err := json.Marshal(struct {
			Set     *evidence.HistoricalReferenceSetV1
			Entries []evidence.HistoricalReferenceEntryV1
		}{group.set, group.entries})
		if err != nil {
			return ""
		}
		parts = append(parts, group.collection, group.slot, strconv.FormatUint(group.id, 10), strconv.Itoa(int(group.pk.Type)), string(group.pk.Value), string(encoded))
	}
	for _, a := range r.attachments {
		encoded, err := json.Marshal(struct {
			Entry   evidence.HistoricalReferenceEntryV1
			Content evidence.Digest
		}{a.entry, a.content})
		if err != nil {
			return ""
		}
		parts = append(parts, a.collection, a.slot, strconv.FormatUint(a.id, 10), string(encoded))
	}
	return mongoOwnerHashParts(parts...)
}

func freezeMongoHistoricalComponentCASRecipe(ctx context.Context, joint *WholeSourceJointPage, p *MongoHistoricalBatchCASPlan) (*mongoHistoricalComponentCASRecipe, error) {
	if p == nil {
		return nil, nil
	}
	if joint == nil || joint.mongo == nil || joint.mongo.global == nil || joint.ValidateBorrowedSnapshot(ctx) != nil || historicalCASComponentPageAlive(ctx, joint) != nil || p.db != joint.mongo.global.db || p.originalSQLConnection != joint.mongo.sqlConnection || p.oldTxn.number != joint.mongo.global.txn.number || !bytes.Equal(p.oldTxn.session, joint.mongo.global.txn.session) || !reflect.DeepEqual(p.before, joint.mongo.data) || !time.Now().Before(p.expires) || len(p.groups) == 0 || len(p.groups) > 512 || len(p.attachments) == 0 || len(p.attachments) > 512 {
		return nil, ErrMongoBatchCAS
	}
	r := &mongoHistoricalComponentCASRecipe{identity: p.identity, sqlIdentity: p.originalSQLIdentity, sqlRows: p.originalSQLRows}
	for _, group := range p.groups {
		g := mongoCASGroup{collection: group.collection, slot: group.slot, id: group.id, pk: bson.RawValue{Type: group.pk.Type, Value: append([]byte(nil), group.pk.Value...)}, set: group.set.Clone()}
		for _, entry := range group.entries {
			if entry.Validate() != nil {
				return nil, ErrMongoBatchCAS
			}
			g.entries = append(g.entries, entry.Clone())
		}
		r.groups = append(r.groups, g)
	}
	for _, a := range p.attachments {
		r.attachments = append(r.attachments, mongoCASAttachmentFacts{a.collection, a.slot, a.id, a.entry.Clone(), a.content})
	}
	if r.digest() == "" || joint.ValidateBorrowedSnapshot(ctx) != nil || historicalCASComponentPageAlive(ctx, joint) != nil {
		return nil, ErrMongoBatchCAS
	}
	return r, nil
}

// First authenticate the COMPLETE original plan. A fragment never fabricates
// a partial live plan or substitutes target IDs for a negative range.
func validateMongoHistoricalComponentOriginalPlan(ctx context.Context, joint *WholeSourceJointPage, p *MongoHistoricalBatchCASPlan) error {
	if p == nil {
		return nil
	}
	if joint == nil || joint.mongo == nil || joint.mongo.global == nil || joint.ValidateBorrowedSnapshot(ctx) != nil || historicalCASComponentPageAlive(ctx, joint) != nil || p.db != joint.mongo.global.db || p.config != joint.mongo.global.config || p.originalSQLConnection != joint.mongo.sqlConnection || p.oldTxn.number != joint.mongo.global.txn.number || !bytes.Equal(p.oldTxn.session, joint.mongo.global.txn.session) || p.metadataHash != joint.mongo.global.metadata.hash || p.identity != joint.mongo.global.metadata.identity || p.originalSQLIdentity != joint.mongo.sql.Report().DatabaseIdentitySHA256 || p.originalSQLRows != joint.mongo.sql.Report().BusinessRowsSHA256 || !reflect.DeepEqual(p.selection, joint.mongo.selection) || !reflect.DeepEqual(p.before, joint.mongo.data) || !time.Now().Before(p.expires) || len(p.groups) == 0 || len(p.groups) > 512 || len(p.attachments) == 0 || len(p.attachments) > 512 {
		return ErrMongoBatchCAS
	}
	return nil
}

func freezeMongoHistoricalOwnerComponentCASRecipe(ctx context.Context, joint *WholeSourceJointPage, p *MongoHistoricalBatchCASPlan, batch *MongoHistoricalOwnerBatch, selected map[string]bool) (*mongoHistoricalComponentCASRecipe, error) {
	if p == nil {
		return nil, nil
	}
	if validateMongoHistoricalComponentOriginalPlan(ctx, joint, p) != nil || batch == nil || batch.ValidateBorrowedSnapshot(ctx) != nil || batch.global != joint.mongo.global || batch.sql != joint.sql.facts || len(selected) == 0 {
		return nil, ErrMongoBatchCAS
	}
	r := &mongoHistoricalComponentCASRecipe{identity: p.identity, sqlIdentity: p.originalSQLIdentity, sqlRows: p.originalSQLRows}
	for _, group := range p.groups {
		raw, err := mongoCASRow(batch.data, group.collection, group.id)
		if err != nil {
			for _, entry := range group.entries {
				if selected[entry.EventID] {
					return nil, ErrMongoBatchConflict
				}
			}
			continue
		}
		if !raw.Lookup("_id").Equal(group.pk) {
			return nil, ErrMongoBatchConflict
		}
		var entries []evidence.HistoricalReferenceEntryV1
		for _, entry := range group.entries {
			// A shared writable group must remain complete. The caller needs
			// the real wider owner/replay closure, never a cut set of entries.
			if entry.Validate() != nil || !selected[entry.EventID] {
				return nil, ErrMongoBatchCAS
			}
			entries = append(entries, entry.Clone())
		}
		r.groups = append(r.groups, mongoCASGroup{collection: group.collection, slot: group.slot, id: group.id, pk: bson.RawValue{Type: group.pk.Type, Value: append([]byte(nil), group.pk.Value...)}, set: group.set.Clone(), entries: entries})
	}
	for _, a := range p.attachments {
		if !selected[a.entry.EventID] {
			continue
		}
		if _, err := mongoCASRow(batch.data, a.collection, a.id); err != nil || a.entry.Validate() != nil {
			return nil, ErrMongoBatchCAS
		}
		r.attachments = append(r.attachments, mongoCASAttachmentFacts{a.collection, a.slot, a.id, a.entry.Clone(), a.content})
	}
	if len(r.attachments) == 0 && len(r.groups) == 0 {
		return nil, nil
	}
	if len(r.attachments) == 0 || len(r.groups) == 0 || r.digest() == "" || batch.ValidateBorrowedSnapshot(ctx) != nil || validateMongoHistoricalComponentOriginalPlan(ctx, joint, p) != nil {
		return nil, ErrMongoBatchCAS
	}
	return r, nil
}

// The statement is an actual physical sub-store effect, not a commit receipt.
// Production entrypoints must first supply the separate opaque fresh source,
// SQL and AI composition. The effect function below is deliberately PRIVATE.
type MongoHistoricalComponentStatement struct {
	self      *MongoHistoricalComponentStatement
	physical  *MongoHistoricalComponentObservation
	statement *MongoHistoricalBatchCASStatement
	expected  map[string][]bson.Raw
	seal      string
}

type MongoHistoricalComponentStatementSummary struct {
	Protocol, ExpectedRowsSHA256, NativeTransactionSHA256              string
	StatementApplied, FullSourcesRequired, SQLQualificationRequired    bool
	AIClosureRequired, HostCommitRequired, IndependentReadbackRequired bool
	HostCommitVerified, BusinessClosureVerified, DropReady             bool
}

func (*MongoHistoricalComponentStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalComponentStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalComponentStatement) String() string {
	return "private physical Mongo component statement; source/SQL/AI/commit unproven"
}

func mongoHistoricalComponentMergedPlan(o *MongoHistoricalComponentObservation) (*MongoHistoricalBatchCASPlan, error) {
	if o == nil || o.component == nil || len(o.frames) != len(o.component.inputs) {
		return nil, ErrMongoBatchCAS
	}
	read := o.component.inputs[0].mongoRead
	p := &MongoHistoricalBatchCASPlan{db: o.db, config: read.config, oldTxn: read.original, metadataHash: o.metadata, identity: read.config.ExpectedIdentityHash, selection: mongoCASCloneSelection(mongoBatchSelection{}), limits: DefaultMongoHistoricalOwnerBatchLimits(), hints: map[string]string{}, expires: o.started.Add(o.budget), before: map[string][]bson.Raw{}}
	// Bounds are the prechecked component bounds, not an extension of any old
	// capability. This NEW plan is usable only by the current native epoch.
	p.limits.MaxRows, p.limits.MaxBytes, p.limits.MaxDuration = 131072, 256<<20, o.budget
	seenRows := map[historicalCASRowKey]bson.Raw{}
	groupPositions := map[string]int{}
	byEvent, bySource := map[string]mongoCASAttachmentFacts{}, map[string]mongoCASAttachmentFacts{}
	var sqlRows []string
	register := func(a mongoCASAttachmentFacts) error {
		key := a.entry.Source.Database + ":" + a.entry.Source.Object + ":" + a.entry.Source.PrimaryKeySHA256
		if old, ok := byEvent[a.entry.EventID]; ok && (old.collection != a.collection || old.id != a.id || !reflect.DeepEqual(old.entry, a.entry)) {
			return evidence.ErrHistoricalReferenceConflict
		}
		if old, ok := bySource[key]; ok && (old.collection != a.collection || old.id != a.id || !reflect.DeepEqual(old.entry, a.entry)) {
			return evidence.ErrHistoricalReferenceConflict
		}
		byEvent[a.entry.EventID], bySource[key] = a, a
		return nil
	}
	for i, f := range o.component.inputs {
		r := f.mongoRead
		if r == nil || r.config != read.config || r.metadata != o.metadata {
			return nil, ErrMongoBatchConflict
		}
		for j, query := range mongoCASSelections(r.selection) {
			target := mongoCASSelections(p.selection)[j]
			for id := range query.ids {
				target.ids[id] = true
			}
			if len(query.ids) > 0 {
				key := query.name + ":" + query.field
				if old := p.hints[key]; old != "" && old != r.hints[key] {
					return nil, ErrMongoBatchConflict
				}
				p.hints[key] = r.hints[key]
			}
		}
		for name, raws := range o.frames[i] {
			for _, raw := range raws {
				id, err := historicalSpoolMongoID(raw)
				key := historicalCASRowKey{"mongodb", name, id}
				if err != nil {
					return nil, err
				}
				if prior := seenRows[key]; prior != nil {
					if !bytes.Equal(prior, raw) {
						return nil, ErrMongoBatchConflict
					}
					continue
				}
				copyRaw := append(bson.Raw(nil), raw...)
				seenRows[key] = copyRaw
				p.before[name] = append(p.before[name], copyRaw)
			}
		}
	}
	for _, name := range mongoBatchBusinessCollections {
		if _, ok := p.before[name]; !ok {
			p.before[name] = nil
		}
	}
	mongoCASSort(p.before)
	for _, location := range []struct{ name, slot string }{{"answersheets", "legacy_submission_evidence"}, {"report_generations", "historical_generated_evidence"}} {
		for _, raw := range p.before[location.name] {
			set, err := mongoCASSet(raw, location.slot)
			if err != nil {
				return nil, err
			}
			id, err := historicalSpoolMongoID(raw)
			if err != nil {
				return nil, err
			}
			if set != nil {
				for _, entry := range set.Entries {
					if err = register(mongoCASAttachmentFacts{collection: location.name, slot: location.slot, id: id, entry: entry}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	for _, f := range o.component.inputs {
		r := f.mongoCAS
		if r == nil {
			continue
		}
		if r.identity != p.identity || p.originalSQLIdentity != "" && r.sqlIdentity != p.originalSQLIdentity {
			return nil, ErrMongoBatchConflict
		}
		p.originalSQLIdentity = r.sqlIdentity
		sqlRows = append(sqlRows, r.sqlRows)
		for _, group := range r.groups {
			key := mongoCASKey(group.collection, group.id) + ":" + group.slot
			position, exists := groupPositions[key]
			if !exists {
				raw, err := mongoCASRow(p.before, group.collection, group.id)
				if err != nil || !raw.Lookup("_id").Equal(group.pk) {
					return nil, ErrMongoBatchConflict
				}
				set, err := mongoCASSet(raw, group.slot)
				if err != nil {
					return nil, err
				}
				position = len(p.groups)
				p.groups = append(p.groups, mongoCASGroup{collection: group.collection, slot: group.slot, id: group.id, pk: group.pk, set: set})
				groupPositions[key] = position
			}
			g := &p.groups[position]
			for _, entry := range group.entries {
				if entry.Validate() != nil || register(mongoCASAttachmentFacts{collection: group.collection, slot: group.slot, id: group.id, entry: entry}) != nil {
					return nil, ErrMongoBatchConflict
				}
				next, err := g.set.Append(entry)
				encoded, marshalErr := bson.Marshal(next)
				if err != nil || marshalErr != nil || len(encoded) > evidence.HistoricalReferenceMaxBytes {
					return nil, ErrMongoBatchBounds
				}
				g.set, g.entries = next, append(g.entries, entry.Clone())
			}
		}
		for _, a := range r.attachments {
			if register(a) != nil {
				return nil, ErrMongoBatchConflict
			}
			already := false
			for _, prior := range p.attachments {
				if prior.entry.EventID == a.entry.EventID {
					if !reflect.DeepEqual(prior, a) {
						return nil, ErrMongoBatchConflict
					}
					already = true
				}
			}
			if !already {
				p.attachments = append(p.attachments, a)
			}
		}
	}
	if len(p.groups) == 0 || len(p.groups) > 512 || len(p.attachments) == 0 || len(p.attachments) > 512 {
		return nil, ErrMongoBatchBounds
	}
	p.originalSQLRows = mongoOwnerHashParts(sqlRows...)
	return p, nil
}

// PRIVATE physical effect. No production caller can obtain write authority
// from frozen inputs alone. A forthcoming typed source/SQL/AI composition must
// call this only after its own actual fresh qualifications succeed.
func applyMongoHistoricalComponent(ctx context.Context, o *MongoHistoricalComponentObservation) (*MongoHistoricalComponentStatement, error) {
	if o == nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	o.applyMu.Lock()
	defer o.applyMu.Unlock()
	if !o.complete || o.applied || o.validate(ctx) != nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	o.applied = true // Any effect or unknown result poisons this instance.
	p, err := mongoHistoricalComponentMergedPlan(o)
	if err != nil {
		return nil, err
	}
	scope, cancel := context.WithDeadline(ctx, o.started.Add(o.budget))
	defer cancel()
	physical, err := p.Apply(mongo.NewSessionContext(scope, o.session))
	if err != nil {
		return nil, err
	}
	if o.validate(ctx) != nil {
		return nil, ErrMongoHistoricalComponentEpoch
	}
	s := &MongoHistoricalComponentStatement{physical: o, statement: physical, expected: mongoCASCloneData(physical.expected)}
	s.self, s.seal = s, mongoCASHash(s.expected, o.metadata)
	return s, nil
}

func (s *MongoHistoricalComponentStatement) Summary() MongoHistoricalComponentStatementSummary {
	r := MongoHistoricalComponentStatementSummary{Protocol: "mongo-component-physical-statement/v1", FullSourcesRequired: true, SQLQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true, IndependentReadbackRequired: true}
	if s == nil || s.self != s || s.physical == nil || s.statement == nil || s.seal == "" || s.seal != mongoCASHash(s.expected, s.physical.metadata) {
		return r
	}
	r.StatementApplied, r.ExpectedRowsSHA256 = true, s.seal
	r.NativeTransactionSHA256 = mongoOwnerHashParts(string(s.statement.transaction.session), strconv.FormatInt(s.statement.transaction.number, 10))
	return r
}

// Actual expected-image reread in a DIFFERENT borrowed host transaction. This
// creates its own bounded physical read; it never renews the old qualification
// and never infers the host Commit result from expected bytes or a boolean.
func (s *MongoHistoricalComponentStatement) VerifyIndependentPersisted(parent context.Context, db *mongo.Database, budget time.Duration) error {
	if parent == nil || parent.Err() != nil || s == nil || s.self != s || s.physical == nil || s.statement == nil || db != s.physical.db || s.seal == "" || s.seal != mongoCASHash(s.expected, s.physical.metadata) || budget <= 0 || budget > 20*time.Second {
		return ErrMongoHistoricalComponentEpoch
	}
	txn, err := mongoCycleTransaction(parent, db)
	if err != nil || mongo.SessionFromContext(parent) == s.physical.session || txn.number == s.statement.transaction.number && bytes.Equal(txn.session, s.statement.transaction.session) {
		return ErrMongoCycleTransaction
	}
	started := time.Now()
	scope, cancel := context.WithDeadline(parent, started.Add(budget))
	defer cancel()
	ctx := mongo.NewSessionContext(scope, mongo.SessionFromContext(parent))
	p := s.statement.plan
	before, err := observeMongoCycleMetadata(ctx, db, p.config)
	if err != nil || before.hash != p.metadataHash {
		return ErrMongoBatchConflict
	}
	actual, err := p.capture(ctx)
	if err != nil || !reflect.DeepEqual(actual, s.expected) {
		return ErrMongoBatchConflict
	}
	after, err := observeMongoCycleMetadata(ctx, db, p.config)
	current, nativeErr := mongoCycleTransaction(ctx, db)
	if err != nil || after.hash != before.hash || nativeErr != nil || current.number != txn.number || !bytes.Equal(current.session, txn.session) || !time.Now().Before(started.Add(budget)) || s.seal != mongoCASHash(s.expected, s.physical.metadata) {
		return ErrMongoHistoricalComponentEpoch
	}
	return nil
}
