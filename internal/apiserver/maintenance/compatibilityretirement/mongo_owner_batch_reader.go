package retirement

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"time"

	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"go.mongodb.org/mongo-driver/bson"
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

// Indexes are actual immutable metadata from the complete global scan. No
// partial/sparse/collated index is guessed to cover the selected original IDs.
func (b *MongoHistoricalOwnerBatch) index(name, field string, unique bool) (string, error) {
	d, ok := b.global.metadata.definitions[name]
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

func (b *MongoHistoricalOwnerBatch) capture(ctx context.Context) error {
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
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
	parts := []string{"mongo-business-owner-rows/v1", b.global.metadata.identity, b.global.metadata.hash}
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
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	// Metadata commands occur outside the transaction, exactly as in the
	// global scanner. Bracket this business page with its same-client anchor.
	end, err := observeMongoCycleMetadata(ctx, b.global.db, b.global.config)
	if err != nil {
		return err
	}
	if end.hash != b.global.metadata.hash {
		return ErrMongoBatchConflict
	}
	return nil
}

func (b *MongoHistoricalOwnerBatch) fetch(ctx context.Context, name, field string, ids map[uint64]bool) error {
	if len(ids) == 0 {
		return nil
	}
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
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
	cur, err := b.global.db.Collection(name).Find(ctx, filter, options.Find().SetHint(index).SetCollation(&options.Collation{Locale: "simple"}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(remaining+1)).SetBatchSize(128).SetMaxTime(15*time.Second).SetComment("qs_retirement_mongo_business_batch_v1"))
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
	return b.ValidateBorrowedSnapshot(ctx)
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
