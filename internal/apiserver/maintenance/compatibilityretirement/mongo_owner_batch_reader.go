package retirement

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"time"

	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoBatchBusinessCollections = []string{"answersheets", "report_generations", "interpret_report_artifacts", "interpretation_runs"}

func mongoBatchPOType(name string) reflect.Type {
	switch name {
	case "answersheets":
		return reflect.TypeOf(sheetmongo.AnswerSheetPO{})
	case "report_generations":
		return reflect.TypeOf(interpretmongo.ReportGenerationPO{})
	case "interpret_report_artifacts":
		return reflect.TypeOf(interpretmongo.InterpretReportPO{})
	case "interpretation_runs":
		return reflect.TypeOf(interpretmongo.InterpretationRunPO{})
	}
	return nil
}

// The original transaction and the distinct read-only snapshot input borrow
// the same bounded query/expansion core with their own strict native guards.
type mongoOwnerReadCore struct {
	db        *mongo.Database
	config    MongoOwnerConfig
	metadata  mongoCycleMetadata
	limits    MongoHistoricalOwnerBatchLimits
	selection mongoBatchSelection
	data      map[string][]bson.Raw
	indexes   map[string]map[string]map[uint64][]bson.Raw
	seen      map[string]map[string]bson.Raw
	report    MongoHistoricalOwnerBatchReport
	validate  func(context.Context) error
}

func (b *MongoHistoricalOwnerBatch) capture(ctx context.Context) error {
	core := &mongoOwnerReadCore{db: b.global.db, config: b.global.config, metadata: b.global.metadata, limits: b.limits, selection: b.selection, data: b.data, indexes: b.indexes, seen: b.seen, report: b.report, validate: b.ValidateBorrowedSnapshot}
	if err := core.capture(ctx); err != nil {
		return err
	}
	b.selection, b.data, b.indexes, b.seen, b.report = core.selection, core.data, core.indexes, core.seen, core.report
	return nil
}
func (b *MongoHistoricalOwnerBatch) index(name, field string, unique bool) (string, error) {
	return (&mongoOwnerReadCore{metadata: b.global.metadata}).index(name, field, unique)
}

// Indexes are actual immutable metadata from the complete global scan. No
// partial/sparse/collated index is guessed to cover the selected original IDs.
func (b *mongoOwnerReadCore) index(name, field string, unique bool) (string, error) {
	d, ok := b.metadata.definitions[name]
	if !ok {
		return "", ErrMongoBatchConflict
	}
	for _, raw := range d.indexes {
		f, err := exactBSONFields(raw)
		if err != nil {
			return "", ErrMongoCycleSchema
		}
		key, ok := f["key"].DocumentOK()
		if !ok {
			return "", ErrMongoCycleSchema
		}
		elements, err := key.Elements()
		if err != nil {
			return "", ErrMongoCycleSchema
		}
		if len(elements) == 0 || elements[0].Key() != field {
			continue
		}
		direction, valid := mongoExactInteger(elements[0].Value())
		if !valid || direction != 1 && direction != -1 {
			continue
		}
		if _, present := f["partialFilterExpression"]; present {
			continue
		}
		if sparse, present := f["sparse"]; present {
			value, valid := sparse.BooleanOK()
			if !valid || value {
				continue
			}
		}
		if c, present := f["collation"]; present {
			if c.Type != bson.TypeEmbeddedDocument || c.Document().Lookup("locale").Type != bson.TypeString || c.Document().Lookup("locale").StringValue() != "simple" {
				continue
			}
		}
		if unique {
			value, valid := f["unique"].BooleanOK()
			if !valid || !value || len(elements) != 1 {
				continue
			}
		}
		if f["name"].Type != bson.TypeString {
			return "", ErrMongoCycleSchema
		}
		return f["name"].StringValue(), nil
	}
	return "", ErrMongoCycleSchema
}

func (b *mongoOwnerReadCore) capture(ctx context.Context) error {
	if err := b.validate(ctx); err != nil {
		return err
	}
	for _, name := range mongoBatchBusinessCollections {
		b.data[name] = nil
		b.indexes[name] = map[string]map[uint64][]bson.Raw{}
		b.seen[name] = map[string]bson.Raw{}
		for _, field := range []string{"domain_id", "outcome_id", "generation_id"} {
			b.indexes[name][field] = map[uint64][]bson.Raw{}
		}
		if _, err := b.index(name, "domain_id", true); err != nil {
			return err
		}
	}
	if _, err := b.index("interpret_report_artifacts", "generation_id", true); err != nil {
		return err
	}
	if err := b.fetch(ctx, "answersheets", "domain_id", b.selection.sheets); err != nil {
		return err
	}
	if err := b.fetch(ctx, "report_generations", "outcome_id", b.selection.outcomes); err != nil {
		return err
	}
	if err := b.fetch(ctx, "report_generations", "domain_id", b.selection.generations); err != nil {
		return err
	}
	for _, raw := range b.data["report_generations"] {
		fields, _ := exactBSONFields(raw)
		id, err := mongoCycleID(fields, "domain_id")
		if err != nil {
			return err
		}
		b.selection.generations[id] = true
		for _, v := range []struct {
			field  string
			target map[uint64]bool
		}{{"outcome_id", b.selection.outcomes}, {"latest_run_id", b.selection.runs}, {"report_id", b.selection.artifacts}} {
			if value, present := fields[v.field]; present {
				n, ok := mongoExactInteger(value)
				if !ok || n < 0 {
					return ErrMongoCycleSchema
				}
				if n > 0 {
					v.target[uint64(n)] = true
				}
			}
		}
	}
	if err := b.fetch(ctx, "interpret_report_artifacts", "outcome_id", b.selection.outcomes); err != nil {
		return err
	}
	if err := b.fetch(ctx, "interpret_report_artifacts", "generation_id", b.selection.generations); err != nil {
		return err
	}
	if err := b.fetch(ctx, "interpret_report_artifacts", "domain_id", b.selection.artifacts); err != nil {
		return err
	}
	for _, raw := range b.data["interpret_report_artifacts"] {
		fields, _ := exactBSONFields(raw)
		id, err := mongoCycleID(fields, "interpretation_run_id")
		if err != nil {
			return err
		}
		b.selection.runs[id] = true
	}
	if err := b.fetch(ctx, "interpretation_runs", "generation_id", b.selection.generations); err != nil {
		return err
	}
	if err := b.fetch(ctx, "interpretation_runs", "domain_id", b.selection.runs); err != nil {
		return err
	}
	parts := []string{"mongo-business-owner-rows/v1", b.metadata.identity, b.metadata.hash}
	for _, name := range mongoBatchBusinessCollections {
		// Sorting only stored byte strings for the page digest does not create
		// a database cursor/token or replace actual BSON server ordering.
		sort.Slice(b.data[name], func(i, j int) bool { return bytes.Compare(b.data[name][i], b.data[name][j]) < 0 })
		parts = append(parts, name)
		for _, raw := range b.data[name] {
			parts = append(parts, string(raw))
		}
	}
	b.report.BusinessRowsSHA256 = mongoOwnerHashParts(parts...)
	if err := b.validate(ctx); err != nil {
		return err
	}
	// Metadata commands occur outside the transaction, exactly as in the
	// global scanner. Bracket this business page with its same-client anchor.
	end, err := observeMongoCycleMetadata(ctx, b.db, b.config)
	if err != nil {
		return err
	}
	if end.hash != b.metadata.hash {
		return ErrMongoBatchConflict
	}
	return nil
}

func (b *mongoOwnerReadCore) fetch(ctx context.Context, name, field string, ids map[uint64]bool) error {
	if len(ids) == 0 {
		return nil
	}
	if err := b.validate(ctx); err != nil {
		return err
	}
	index, err := b.index(name, field, field == "domain_id")
	if err != nil {
		return err
	}
	values := bson.A{}
	for _, id := range mongoBatchIDs(ids) {
		if id == 0 || id > 1<<63-1 {
			return ErrMongoBatchInvalid
		}
		values = append(values, int64(id))
	}
	if len(values) > b.limits.MaxRows {
		return ErrMongoBatchBounds
	}
	filter := bson.D{{Key: field, Value: bson.D{{Key: "$in", Value: values}}}}
	remaining := b.limits.MaxRows - int(b.report.ReadRows)
	if remaining <= 0 {
		return ErrMongoBatchBounds
	}
	cur, err := b.db.Collection(name).Find(ctx, filter, options.Find().SetHint(index).SetCollation(&options.Collation{Locale: "simple"}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(remaining+1)).SetBatchSize(128).SetMaxTime(15*time.Second).SetComment("qs_retirement_mongo_business_batch_v1"))
	if err != nil {
		return ErrMongoOwnerRead
	}
	defer func() { _ = cur.Close(ctx) }()
	b.report.Queries++
	for cur.Next(ctx) {
		raw := append(bson.Raw(nil), cur.Current...)
		b.report.ReadRows++
		b.report.ReadBytes += uint64(len(raw))
		if b.report.ReadRows > uint64(b.limits.MaxRows) || b.report.ReadBytes > b.limits.MaxBytes || len(raw) > MaxSourceRowBytes {
			return ErrMongoBatchBounds
		}
		if mongoUniqueBSON(raw, 0) != nil || mongoPOShape(raw, mongoBatchPOType(name)) != nil {
			return ErrMongoCycleSchema
		}
		fields, err := exactBSONFields(raw)
		if err != nil {
			return err
		}
		id, err := mongoCycleID(fields, "domain_id")
		if err != nil {
			return err
		}
		selected, err := mongoCycleID(fields, field)
		if err != nil || !ids[selected] {
			return ErrMongoBatchConflict
		}
		pk, exists := fields["_id"]
		if !exists || pk.Type != bson.TypeObjectID {
			return ErrMongoCycleSchema
		}
		pkBytes := string(pk.Value)
		if old, found := b.seen[name][pkBytes]; found {
			if !bytes.Equal(old, raw) {
				return ErrMongoBatchConflict
			}
			continue
		}
		b.seen[name][pkBytes] = raw
		b.data[name] = append(b.data[name], raw)
		b.report.UniqueBusinessRows++
		for _, key := range []string{"domain_id", "outcome_id", "generation_id"} {
			if value, present := fields[key]; present {
				n, ok := mongoExactInteger(value)
				if !ok || n < 0 {
					return ErrMongoCycleSchema
				}
				if n > 0 {
					b.indexes[name][key][uint64(n)] = append(b.indexes[name][key][uint64(n)], raw)
				}
			}
		}
		if len(b.indexes[name]["domain_id"][id]) != 1 {
			return ErrMongoBatchConflict
		}
	}
	if cur.Err() != nil {
		return ErrMongoOwnerRead
	}
	if err = cur.Close(ctx); err != nil {
		return ErrMongoOwnerRead
	}
	return b.validate(ctx)
}

// Only the exact selectors used by the shared original business verifier are
// supported. No generic BSON filter interpreter or DB fallback is available.
func (b *MongoHistoricalOwnerBatch) rows(ctx context.Context, name string, filter bson.D) ([]bson.Raw, error) {
	if err := b.validateReaderContext(ctx); err != nil {
		return nil, err
	}
	idx, exists := b.indexes[name]
	if !exists || len(filter) != 1 {
		return nil, ErrMongoBatchInvalid
	}
	var predicates []bson.D
	if filter[0].Key == "$or" {
		list, ok := filter[0].Value.(bson.A)
		if !ok || len(list) != 2 {
			return nil, ErrMongoBatchInvalid
		}
		for _, v := range list {
			predicate, ok := v.(bson.D)
			if !ok {
				return nil, ErrMongoBatchInvalid
			}
			predicates = append(predicates, predicate)
		}
	} else {
		predicates = []bson.D{filter}
	}
	var out []bson.Raw
	seen := map[string]bool{}
	for _, predicate := range predicates {
		if len(predicate) != 1 {
			return nil, ErrMongoBatchInvalid
		}
		field := predicate[0].Key
		rows, ok := idx[field]
		if !ok {
			return nil, ErrMongoBatchInvalid
		}
		var id uint64
		switch v := predicate[0].Value.(type) {
		case int64:
			if v > 0 {
				id = uint64(v)
			}
		case int32:
			if v > 0 {
				id = uint64(v)
			}
		default:
			return nil, ErrMongoBatchInvalid
		}
		if id == 0 {
			return nil, ErrMongoBatchInvalid
		}
		for _, raw := range rows[id] {
			pk := string(raw.Lookup("_id").Value)
			if !seen[pk] {
				seen[pk] = true
				out = append(out, append(bson.Raw(nil), raw...))
			}
		}
	}
	return out, nil
}

func (b *MongoHistoricalOwnerBatch) artifactIndexes(ctx context.Context) ([]bson.Raw, error) {
	if err := b.validateReaderContext(ctx); err != nil {
		return nil, err
	}
	d, ok := b.global.metadata.definitions["interpret_report_artifacts"]
	if !ok {
		return nil, ErrMongoBatchConflict
	}
	rows := make([]bson.Raw, len(d.indexes))
	for i, raw := range d.indexes {
		rows[i] = append(bson.Raw(nil), raw...)
	}
	return rows, nil
}

// MongoSnapshotOwnerFootprint is immutable planning INPUT. It cannot be used
// as a MongoHistoricalOwnerBatch, global graph, source closure or CAS permit.
type MongoSnapshotOwnerFootprint struct {
	self                                         *MongoSnapshotOwnerFootprint
	inputSHA, metadataSHA, nativeSHA, sqlRowsSHA string
	sources                                      map[verifiedSourceKey]*DecodedSourceEvent
	sourceFactsSHA                               map[verifiedSourceKey][32]byte
	sqlOwners                                    map[verifiedSourceKey]sqlevaluation.SQLHistoricalFactsSnapshot
	sqlAbsent                                    map[verifiedSourceKey]bool
	selection                                    mongoBatchSelection
	data                                         map[string][]bson.Raw
}

func (*MongoSnapshotOwnerFootprint) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoSnapshotOwnerFootprint) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoSnapshotOwnerFootprint) String() string {
	return "private immutable owner input; no write authority"
}
func (f *MongoSnapshotOwnerFootprint) GoString() string { return f.String() }
func (f *MongoSnapshotOwnerFootprint) InputSHA256() string {
	if f == nil || f.self != f {
		return ""
	}
	return f.inputSHA
}
func (f *MongoSnapshotOwnerFootprint) VisitRows(ctx context.Context, visit func(string, bson.Raw) error) error {
	if f == nil || f.self != f || ctx == nil || ctx.Err() != nil || visit == nil {
		return ErrMongoBatchInvalid
	}
	for _, name := range mongoBatchBusinessCollections {
		for _, raw := range f.data[name] {
			if err := visit(name, append(bson.Raw(nil), raw...)); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

// The host still owns the original SQL RRRO transaction and the actual
// nontransaction Mongo snapshot session. Both must remain alive throughout
// this bounded read; neither is retained as a renewable authority afterwards.
func PrepareMongoSnapshotOwnerFootprint(ctx context.Context, input *MongoSnapshotInputEpoch, sql *sqlevaluation.SQLHistoricalOwnerBatch, sources []*VerifiedSourceEvent, limits MongoHistoricalOwnerBatchLimits) (*MongoSnapshotOwnerFootprint, error) {
	if ctx == nil || ctx.Err() != nil || input == nil || sql == nil || !input.complete || !sql.Report().Complete || !limits.valid() || len(sources) == 0 || len(sources) > limits.MaxSources {
		return nil, ErrMongoBatchInvalid
	}
	started := time.Now()
	ctx, cancel := context.WithDeadline(ctx, started.Add(limits.MaxDuration))
	defer cancel()
	validate := func(ctx context.Context) error {
		if time.Since(started) > limits.MaxDuration {
			return ErrMongoBatchBounds
		}
		if err := input.ValidateBorrowedInputEpoch(ctx); err != nil {
			return err
		}
		return sql.ValidateBorrowedSnapshot(ctx)
	}
	if err := validate(ctx); err != nil {
		return nil, err
	}
	selected, err := selectMongoOwnerSources(sql, sources)
	if err != nil {
		return nil, err
	}
	core := &mongoOwnerReadCore{db: input.db, config: input.config, metadata: input.metadata, limits: limits, selection: selected.selection, data: map[string][]bson.Raw{}, indexes: map[string]map[string]map[uint64][]bson.Raw{}, seen: map[string]map[string]bson.Raw{}, validate: validate}
	if err = core.capture(ctx); err != nil {
		return nil, err
	}
	if err = input.matchOwnerRows(ctx, core.data); err != nil {
		return nil, err
	}
	if err = validate(ctx); err != nil {
		return nil, err
	}
	summary := input.Summary()
	f := &MongoSnapshotOwnerFootprint{metadataSHA: input.metadata.hash, nativeSHA: summary.NativeEpochSHA256, sqlRowsSHA: sql.Report().BusinessRowsSHA256, sources: selected.sources, sourceFactsSHA: selected.sourceFactsSHA, sqlOwners: map[verifiedSourceKey]sqlevaluation.SQLHistoricalFactsSnapshot{}, sqlAbsent: selected.sqlAbsent, selection: core.selection, data: core.data}
	f.self = f
	parts := []string{"mongo-snapshot-owner-input/v1", summary.SnapshotSHA256, f.metadataSHA, f.nativeSHA, f.sqlRowsSHA, core.report.BusinessRowsSHA256}
	keys := make([]verifiedSourceKey, 0, len(f.sources))
	for key := range f.sources {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].object < keys[j].object || keys[i].object == keys[j].object && bytes.Compare(keys[i].pk[:], keys[j].pk[:]) < 0
	})
	for _, key := range keys {
		digest := f.sourceFactsSHA[key]
		parts = append(parts, string([]byte{key.object}), string(key.pk[:]), string(digest[:]))
		if owner := selected.sqlOwners[key]; owner != nil {
			snapshot := owner.Snapshot()
			raw, e := historicalSpoolEncode(snapshot)
			if e != nil {
				return nil, e
			}
			f.sqlOwners[key] = snapshot
			parts = append(parts, historicalSpoolSHA(raw))
		} else if f.sqlAbsent[key] {
			parts = append(parts, "actual_sql_sheet_absent")
		} else {
			return nil, ErrMongoBatchConflict
		}
	}
	for _, ids := range []map[uint64]bool{core.selection.sheets, core.selection.outcomes, core.selection.generations, core.selection.artifacts, core.selection.runs} {
		raw, e := historicalSpoolEncode(mongoBatchIDs(ids))
		if e != nil {
			return nil, e
		}
		parts = append(parts, historicalSpoolSHA(raw))
	}
	f.inputSHA = mongoOwnerHashParts(parts...)
	if err = validate(ctx); err != nil {
		return nil, err
	}
	return f, nil
}
