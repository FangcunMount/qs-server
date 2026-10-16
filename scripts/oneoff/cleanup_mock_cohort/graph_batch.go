package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const graphBatchBytes = 64 * 1024 * 1024
const graphBatchParameters = 60000 // below MySQL's 65535 prepared parameters
const graphMongoCommandBytes = 16*1024*1024 - 8192

// Only read-only, pre-claim work may split this size error into smaller batches.
var errGraphBatchBudget = errors.New("exact graph batch exceeds fixed budget")

type graphReadResult struct {
	Record record
	Exists bool
}

func graphBatchObjects(objects []object, writable bool) error {
	if len(objects) == 0 {
		return errors.New("empty exact graph batch")
	}
	first := objects[0]
	limit := 1000
	if first.Store == "mongo" {
		limit = 500
	}
	if len(objects) > limit {
		return fmt.Errorf("%w: exact graph batch exceeds fixed cardinality", errGraphBatchBudget)
	}
	seen, size := map[string]bool{}, 0
	for _, o := range objects {
		size += len(encode(o))
		if size > graphBatchBytes {
			return fmt.Errorf("%w: exact graph keys exceed fixed 64MiB", errGraphBatchBudget)
		}
		if err := validateObject(o); err != nil {
			return err
		}
		if o.Store != first.Store || o.Name != first.Name || seen[objectID(o)] {
			return errors.New("exact graph batch requires one namespace and unique physical keys")
		}
		seen[objectID(o)] = true
		if o.Store == "mysql" {
			rank, ok := graphSQLTables[o.Name]
			if writable && (!ok || rank <= 0) || !writable && !ok && !readableSQL[o.Name] {
				return errors.New("SQL namespace outside fixed writable graph")
			}
			if _, _, err := keyWhere(o); err != nil {
				return err
			}
		} else {
			if rank, ok := graphMongoTables[o.Name]; !ok || writable && rank <= 0 {
				return errors.New("mongo namespace outside fixed graph")
			}
			if _, err := mongoKey(o); err != nil {
				return err
			}
		}
	}
	return nil
}

// Refresh metadata for this call, including inside the SQL transaction. Cached
// discovery metadata cannot prove the physical type or DDL at a later write.
func graphBatchSQLShape(ctx context.Context, q sqlReader, name string) (graphSQLShape, error) {
	var shape graphSQLShape
	rows, err := q.QueryContext(ctx, "SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=k.TABLE_SCHEMA AND c.TABLE_NAME=k.TABLE_NAME AND c.COLUMN_NAME=k.COLUMN_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.CONSTRAINT_NAME='PRIMARY' ORDER BY k.COLUMN_NAME", name)
	if err != nil {
		return shape, err
	}
	for rows.Next() {
		var name, dataType, columnType string
		if err = rows.Scan(&name, &dataType, &columnType); err != nil {
			return shape, errors.Join(err, rows.Close())
		}
		kind := graphSQLKind(dataType, columnType)
		if !keyIdentifier.MatchString(name) || kind == "" {
			return shape, errors.Join(errors.New("unsupported actual graph primary key"), rows.Close())
		}
		shape.keys = append(shape.keys, cell{Name: name, Type: kind})
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return shape, err
	}
	if len(shape.keys) == 0 {
		return shape, errors.New("actual graph primary key missing")
	}
	var table, ddl, zone, charset, mode string
	if err = q.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&table, &ddl); err != nil {
		return shape, err
	}
	if table != name {
		return shape, errors.New("graph namespace metadata name changed")
	}
	if err = q.QueryRowContext(ctx, "SELECT @@session.time_zone,@@session.character_set_results,@@session.sql_mode").Scan(&zone, &charset, &mode); err != nil {
		return shape, err
	}
	shape.definition = encode([]string{ddl, zone, charset, mode})
	return shape, nil
}

func graphBatchPredicate(objects []object, shape graphSQLShape) (string, []any, error) {
	parts, args := []string{}, []any{}
	for _, o := range objects {
		if len(o.Key) != len(shape.keys) {
			return "", nil, errors.New("complete actual graph primary key required")
		}
		for i, key := range o.Key {
			if key.Name != shape.keys[i].Name || key.Type != shape.keys[i].Type {
				return "", nil, errors.New("caller key cannot coerce actual graph primary key")
			}
		}
		where, values, err := keyWhere(o)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, "("+where+")")
		args = append(args, values...)
	}
	if len(args) > graphBatchParameters {
		return "", nil, fmt.Errorf("%w: graph batch exceeds SQL parameter budget before claim", errGraphBatchBudget)
	}
	return strings.Join(parts, " OR "), args, nil
}

func graphReadSQLBatch(ctx context.Context, q sqlReader, objects []object, shape graphSQLShape, lock bool) ([]graphReadResult, error) {
	where, args, err := graphBatchPredicate(objects, shape)
	if err != nil {
		return nil, err
	}
	query := "SELECT x.* FROM `" + objects[0].Name + "` x WHERE " + where
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out, positions := graphEmptyResults(objects, shape.definition)
	seen, size := map[int]bool{}, 0
	for rows.Next() {
		r, err := scanGraphSQLRow(rows, names, types, shape, objects[0].Name)
		if err != nil {
			return nil, err
		}
		position, ok := positions[objectID(r.Object)]
		if !ok || seen[position] {
			return nil, errors.New("SQL query matched a different or repeated physical key")
		}
		seen[position] = true
		size += len(encode(r))
		if size > graphBatchBytes {
			return nil, fmt.Errorf("%w: SQL original batch exceeds fixed 64MiB", errGraphBatchBudget)
		}
		r.Object = objects[position]
		out[position] = graphReadResult{r, true}
	}
	return out, errors.Join(rows.Err(), rows.Close(), graphBatchResultBudget(out))
}

func graphBatchResultBudget(results []graphReadResult) error {
	size := 0
	for _, result := range results {
		size += len(encode(result.Record))
		if size > graphBatchBytes {
			return fmt.Errorf("%w: exact graph results exceed fixed 64MiB", errGraphBatchBudget)
		}
	}
	return nil
}

func graphEmptyResults(objects []object, definition []byte) ([]graphReadResult, map[string]int) {
	out := make([]graphReadResult, len(objects))
	positions := map[string]int{}
	for i, o := range objects {
		out[i].Record = record{Object: o, Definition: definition}
		positions[objectID(o)] = i
	}
	return out, positions
}

func graphMongoBatchFilter(objects []object) (bson.D, error) {
	ids := bson.A{}
	for _, o := range objects {
		key, err := mongoKey(o)
		if err != nil {
			return nil, err
		}
		ids = append(ids, key[0].Value)
	}
	return bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}, nil
}

func (s *graphStore) readGraphMongoBatch(ctx context.Context, objects []object, definition []byte) ([]graphReadResult, error) {
	filter, err := graphMongoBatchFilter(objects)
	if err != nil {
		return nil, err
	}
	cur, err := s.live.mongo.Database(s.live.mongoDB).Collection(objects[0].Name).Find(ctx, filter, options.Find().SetBatchSize(500).SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	out, positions := graphEmptyResults(objects, definition)
	seen, size := map[int]bool{}, 0
	for cur.Next(ctx) {
		r, err := graphMongoObject(objects[0].Name, cur.Current, definition)
		if err != nil {
			return nil, err
		}
		position, ok := positions[objectID(r.Object)]
		if !ok || seen[position] {
			return nil, errors.New("mongo query matched an alias or repeated physical key")
		}
		if err = exactMongoKey(r.BSON, objects[position]); err != nil {
			return nil, err
		}
		seen[position] = true
		size += len(encode(r))
		if size > graphBatchBytes {
			return nil, fmt.Errorf("%w: Mongo original batch exceeds fixed 64MiB", errGraphBatchBudget)
		}
		r.Object = objects[position]
		out[position] = graphReadResult{r, true}
	}
	return out, errors.Join(cur.Err(), cur.Close(ctx), graphBatchResultBudget(out))
}

func (s *graphStore) readExactBatch(ctx context.Context, objects []object) ([]graphReadResult, error) {
	if err := graphBatchObjects(objects, false); err != nil {
		return nil, err
	}
	if objects[0].Store == "mysql" {
		shape, err := graphBatchSQLShape(ctx, s.live.readSQL(), objects[0].Name)
		if err != nil {
			return nil, err
		}
		return graphReadSQLBatch(ctx, s.live.readSQL(), objects, shape, false)
	}
	definition, err := newGraphStore(s.live).mongoDefinition(ctx, objects[0].Name)
	if err != nil {
		return nil, err
	}
	return s.readGraphMongoBatch(ctx, objects, definition)
}

func graphBatchRecords(records []record) ([]object, error) {
	objects, total := make([]object, len(records)), 0
	for i, r := range records {
		objects[i] = r.Object
		rowBytes := len(encode(r))
		total += rowBytes
		if rowBytes > 32*1024*1024 || total > graphBatchBytes {
			return nil, fmt.Errorf("%w: frozen original raw bytes", errGraphBatchBudget)
		}
		if len(r.Definition) == 0 || r.Object.Fingerprint != "" && r.Object.Fingerprint != fingerprint(r) {
			return nil, errors.New("invalid frozen original graph batch")
		}
		if r.Object.Store == "mongo" {
			if len(r.BSON) > graphMongoCommandBytes {
				return nil, fmt.Errorf("%w: single original BSON command", errGraphBatchBudget)
			}
			if len(r.Columns) != 0 || bson.Raw(r.BSON).Validate() != nil {
				return nil, errors.New("invalid original graph BSON")
			}
			if err := exactMongoKey(r.BSON, r.Object); err != nil {
				return nil, err
			}
		} else if len(r.BSON) != 0 || len(r.Columns) == 0 {
			return nil, errors.New("invalid original graph SQL row")
		}
	}
	return objects, graphBatchObjects(objects, true)
}

// Must be called by the engine before its durable claim. Failure requires a
// smaller explicit batch; this provider never silently splits/retries a write.
func (s *graphStore) preflightExactBatch(ctx context.Context, records []record, restore bool) error {
	objects, err := graphBatchRecords(records)
	if err != nil {
		return err
	}
	if objects[0].Store == "mysql" {
		shape, err := graphBatchSQLShape(ctx, s.live.readSQL(), objects[0].Name)
		if err != nil {
			return err
		}
		_, _, err = graphSQLWritePlan(ctx, s.live.readSQL(), records, shape, restore)
		return err
	}
	definition, err := newGraphStore(s.live).mongoDefinition(ctx, objects[0].Name)
	if err != nil {
		return err
	}
	wire := 4096
	for _, r := range records {
		if !bytes.Equal(definition, r.Definition) {
			return errors.New("mongo original namespace/UUID/index definition changed")
		}
		wire += len(r.BSON) + 2*len(encode(r.Object.Key)) + 128
	}
	if wire > graphMongoCommandBytes {
		return fmt.Errorf("%w: graph batch exceeds conservative Mongo command budget before claim", errGraphBatchBudget)
	}
	if restore {
		return s.graphMongoUniquePreflight(ctx, records, definition)
	}
	return nil
}

func graphSQLWritePlan(ctx context.Context, q sqlReader, records []record, shape graphSQLShape, restore bool) (string, []any, error) {
	// A physical PK list is insufficient if a trigger or referential action can
	// mutate another object. The same closed metadata check is made before claim
	// and under the transaction's table metadata lock, before the actual write.
	if err := graphBatchSQLVisibility(ctx, q); err != nil {
		return "", nil, err
	}
	var triggers, cascades uint64
	if err := q.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE=?),(SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE UNIQUE_CONSTRAINT_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME=? AND DELETE_RULE IN ('CASCADE','SET NULL'))", records[0].Object.Name, records[0].Object.Name).Scan(&triggers, &cascades); err != nil {
		return "", nil, err
	}
	if triggers != 0 || cascades != 0 {
		return "", nil, errors.New("SQL trigger or cascading referential write is outside exact graph authority")
	}
	objects := make([]object, len(records))
	for i, r := range records {
		objects[i] = r.Object
		if !bytes.Equal(shape.definition, r.Definition) {
			return "", nil, errors.New("SQL original schema/session definition changed")
		}
	}
	where, keyArgs, err := graphBatchPredicate(objects, shape)
	if err != nil {
		return "", nil, err
	}
	query, args := "DELETE FROM `"+objects[0].Name+"` WHERE "+where, keyArgs
	if restore {
		rows, err := q.QueryContext(ctx, "SELECT * FROM `"+objects[0].Name+"` LIMIT 0")
		if err != nil {
			return "", nil, err
		}
		names, namesErr := rows.Columns()
		types, typesErr := rows.ColumnTypes()
		if err = errors.Join(namesErr, typesErr, rows.Close()); err != nil {
			return "", nil, err
		}
		generated, err := generatedColumns(ctx, q, objects[0].Name)
		if err != nil {
			return "", nil, err
		}
		columns, slots, values := []string{}, []string{}, []string{}
		for _, name := range names {
			if !keyIdentifier.MatchString(name) {
				return "", nil, errors.New("invalid actual column identifier")
			}
			if !generated[name] {
				columns = append(columns, "`"+name+"`")
				slots = append(slots, "?")
			}
		}
		args = []any{}
		for _, r := range records {
			if len(r.Columns) != len(names) {
				return "", nil, errors.New("original row is not the complete actual column set")
			}
			for i, c := range r.Columns {
				if c.Name != names[i] || c.Type != types[i].DatabaseTypeName() || c.Null && c.Raw != nil || !c.Null && c.Raw == nil {
					return "", nil, errors.New("original SQL cell shape differs from actual column")
				}
				if !generated[c.Name] {
					value, err := sqlValue(c)
					if err != nil {
						return "", nil, err
					}
					args = append(args, value)
				}
			}
			values = append(values, "("+strings.Join(slots, ",")+")")
		}
		if len(columns) == 0 {
			return "", nil, errors.New("no restorable original SQL columns")
		}
		query = "INSERT INTO `" + objects[0].Name + "` (" + strings.Join(columns, ",") + ") VALUES " + strings.Join(values, ",")
	}
	var packet int64
	if err = q.QueryRowContext(ctx, "SELECT @@max_allowed_packet").Scan(&packet); err != nil {
		return "", nil, err
	}
	wire := int64(len(query) + 4096 + len(args)*128)
	for _, a := range args {
		if raw, ok := a.([]byte); ok {
			wire += 2 * int64(len(raw))
		}
	}
	if packet <= 0 {
		return "", nil, errors.New("invalid actual SQL packet budget")
	}
	if wire >= packet || wire > graphBatchBytes || len(args) > graphBatchParameters {
		return "", nil, fmt.Errorf("%w: SQL packet/parameter budget before claim", errGraphBatchBudget)
	}
	if restore {
		if err = graphSQLUniquePreflight(ctx, q, records, shape, packet); err != nil {
			return "", nil, err
		}
	}
	return query, args, nil
}

// Read every actual secondary unique constraint before the engine consumes an
// approval or restores an earlier namespace. Equality deliberately follows the
// index's real SQL collation; only the returned physical PK/full original may
// be accepted. BINARY equality here would miss case/padding collisions.
func graphSQLUniquePreflight(ctx context.Context, q sqlReader, records []record, shape graphSQLShape, packet int64) error {
	rows, err := q.QueryContext(ctx, "SELECT INDEX_NAME,SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND NON_UNIQUE=0 ORDER BY INDEX_NAME,SEQ_IN_INDEX", records[0].Object.Name)
	if err != nil {
		return err
	}
	type uniqueKey struct {
		name    string
		columns []string
	}
	keys := []uniqueKey{}
	for rows.Next() {
		var name, indexType string
		var seq int
		var column sql.NullString
		var prefix sql.NullInt64
		if err = rows.Scan(&name, &seq, &column, &prefix, &indexType); err != nil {
			return errors.Join(err, rows.Close())
		}
		if !column.Valid || !keyIdentifier.MatchString(column.String) || prefix.Valid || indexType != "BTREE" {
			return errors.Join(errors.New("unsupported actual SQL unique constraint"), rows.Close())
		}
		if name == "PRIMARY" {
			continue
		}
		if len(keys) == 0 || keys[len(keys)-1].name != name {
			keys = append(keys, uniqueKey{name: name})
		}
		key := &keys[len(keys)-1]
		if seq != len(key.columns)+1 || seq > 16 || len(keys) > 64 {
			return errors.Join(errors.New("invalid actual SQL unique constraint ordering"), rows.Close())
		}
		key.columns = append(key.columns, column.String)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	expected := map[string]record{}
	for _, r := range records {
		expected[objectID(r.Object)] = r
	}
	for _, key := range keys {
		parts, args := []string{}, []any{}
		for _, r := range records {
			conditions, values, skip := []string{}, []any{}, false
			for _, column := range key.columns {
				found := false
				for _, c := range r.Columns {
					if c.Name != column {
						continue
					}
					found = true
					if c.Null {
						skip = true
						break
					}
					value, e := sqlValue(c)
					if e != nil {
						return e
					}
					conditions = append(conditions, "x.`"+column+"` = ?")
					values = append(values, value)
					break
				}
				if !found {
					return errors.New("original SQL row omits an actual unique column")
				}
				if skip {
					break
				}
			}
			if !skip {
				parts = append(parts, "("+strings.Join(conditions, " AND ")+")")
				args = append(args, values...)
			}
		}
		// MySQL permits repeated unique tuples containing SQL NULL.
		if len(parts) == 0 {
			continue
		}
		query := "SELECT x.* FROM `" + records[0].Object.Name + "` x WHERE " + strings.Join(parts, " OR ")
		if _, locked := q.(*sql.Tx); locked {
			query += " FOR UPDATE"
		}
		wire := int64(len(query) + 4096 + len(args)*128)
		for _, arg := range args {
			if b, ok := arg.([]byte); ok {
				wire += 2 * int64(len(b))
			}
		}
		if len(args) > graphBatchParameters || wire >= packet || wire > graphBatchBytes {
			return fmt.Errorf("%w: SQL unique collision probe", errGraphBatchBudget)
		}
		current, e := q.QueryContext(ctx, query, args...)
		if e != nil {
			return e
		}
		names, ne := current.Columns()
		types, te := current.ColumnTypes()
		if e = errors.Join(ne, te); e != nil {
			return errors.Join(e, current.Close())
		}
		count := 0
		for current.Next() {
			count++
			r, e := scanGraphSQLRow(current, names, types, shape, records[0].Object.Name)
			if e != nil {
				return errors.Join(e, current.Close())
			}
			original, ok := expected[objectID(r.Object)]
			if !ok || !sameRecord(r, original) || count > len(records) {
				return errors.Join(errors.New("restore SQL unique constraint conflicts with a foreign or changed original"), current.Close())
			}
		}
		if e = errors.Join(current.Err(), current.Close()); e != nil {
			return e
		}
	}
	return nil
}

type graphMongoUniqueSpec struct {
	keys    []string
	partial bson.D
	mode    string
}

// This is the fixed 1..39 graph catalog, not an index-expression interpreter.
// The six historical partial/multikey indexes retain their original complete
// key/filter/collation contracts; a new or unsupported unique spec stops capture.
func graphMongoUniqueIndexes(definition []byte, name, database string) ([]graphMongoUniqueSpec, error) {
	var metadata struct {
		Collection bson.Raw
		Indexes    []bson.Raw
	}
	if err := json.Unmarshal(definition, &metadata); err != nil {
		return nil, err
	}
	if metadata.Collection.Lookup("name").Type != bson.TypeString || metadata.Collection.Lookup("name").StringValue() != name || len(metadata.Indexes) == 0 {
		return nil, errors.New("actual Mongo unique namespace metadata missing")
	}
	if collation := metadata.Collection.Lookup("options", "collation"); collation.Type != 0 && !graphMongoSimpleCollation(collation) {
		return nil, errors.New("unsupported actual Mongo collection collation")
	}
	plain := map[string][]string{
		"answersheets/idx_domain_id":                                   {"domain_id"},
		"report_generations/uk_generation_domain_id":                   {"domain_id"},
		"report_generations/uk_generation_key":                         {"outcome_id", "report_type", "template_version"},
		"interpretation_runs/uk_interpretation_run_domain_id":          {"domain_id"},
		"interpretation_runs/uk_interpretation_run_generation_attempt": {"generation_id", "attempt"},
		"interpret_report_artifacts/uk_artifact_domain_id":             {"domain_id"},
		"interpret_report_artifacts/uk_artifact_generation_id":         {"generation_id"},
		"report_query_catalog/uk_report_catalog_assessment":            {"assessment_id"},
	}
	partials := map[string]graphMongoUniqueSpec{
		"answersheets/uk_answersheet_durable_event_id":                   {keys: []string{"durable_acceptance.event_id"}, partial: bson.D{{Key: "durable_acceptance.event_id", Value: bson.D{{Key: "$type", Value: "string"}}}}, mode: "durable_event"},
		"answersheets/uk_answersheet_answering_start":                    {keys: []string{"start_context.id"}, partial: bson.D{{Key: "start_context.id", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}, mode: "positive_start"},
		"report_generations/uk_generation_generated_event_id":            {keys: []string{"generated_event_id"}, partial: bson.D{{Key: "generated_event_id", Value: bson.D{{Key: "$type", Value: "string"}, {Key: "$gt", Value: ""}}}}, mode: "nonempty_event"},
		"interpretation_runs/uk_interpretation_run_retry_event_id":       {keys: []string{"retry_event_id"}, partial: bson.D{{Key: "retry_event_id", Value: bson.D{{Key: "$type", Value: "string"}, {Key: "$gt", Value: ""}}}}, mode: "nonempty_event"},
		"answersheets/uk_answersheet_legacy_submission_event_id":         {keys: []string{"legacy_submission_evidence.entries.event_id"}, partial: bson.D{{Key: "legacy_submission_evidence.entries.event_id", Value: bson.D{{Key: "$type", Value: "string"}}}}, mode: "event_entries"},
		"report_generations/uk_generation_historical_generated_event_id": {keys: []string{"historical_generated_evidence.entries.event_id"}, partial: bson.D{{Key: "historical_generated_evidence.entries.event_id", Value: bson.D{{Key: "$type", Value: "string"}}}}, mode: "event_entries"},
	}
	out := []graphMongoUniqueSpec{}
	primary := false
	for _, raw := range metadata.Indexes {
		indexName := raw.Lookup("name")
		if indexName.Type != bson.TypeString {
			return nil, errors.New("actual Mongo index name missing")
		}
		unique := raw.Lookup("unique")
		if unique.Type != 0 && unique.Type != bson.TypeBoolean {
			return nil, errors.New("actual Mongo unique flag is not boolean")
		}
		if indexName.StringValue() != "_id_" && (unique.Type == 0 || !unique.Boolean()) {
			continue
		}
		version := raw.Lookup("v")
		if version.Type != bson.TypeInt32 || version.Int32() != 2 {
			return nil, errors.New("actual Mongo unique index version missing or unsupported")
		}
		elements, err := raw.Elements()
		if err != nil {
			return nil, err
		}
		for _, field := range elements {
			switch field.Key() {
			case "v":
				if field.Value().Type != bson.TypeInt32 || field.Value().Int32() != 2 {
					return nil, errors.New("unsupported Mongo unique index version")
				}
			case "ns":
				if field.Value().Type != bson.TypeString || field.Value().StringValue() != database+"."+name {
					return nil, errors.New("mongo unique index namespace changed")
				}
			case "name", "key", "unique", "partialFilterExpression":
			case "collation":
				if !graphMongoSimpleCollation(field.Value()) {
					return nil, errors.New("unsupported Mongo unique index collation")
				}
			default:
				return nil, errors.New("unsupported actual Mongo unique index option")
			}
		}
		key := raw.Lookup("key")
		if key.Type != bson.TypeEmbeddedDocument {
			return nil, errors.New("mongo unique index key missing")
		}
		if indexName.StringValue() == "_id_" {
			if primary || !graphMongoIndexKeys(key.Document(), []string{"_id"}) || raw.Lookup("partialFilterExpression").Type != 0 {
				return nil, errors.New("actual Mongo primary index changed")
			}
			primary = true
			continue // Physical _id and original bytes are already checked separately.
		}
		id := name + "/" + indexName.StringValue()
		spec, partial := partials[id]
		if !partial {
			keys, ok := plain[id]
			if !ok || raw.Lookup("partialFilterExpression").Type != 0 {
				return nil, errors.New("unsupported actual Mongo unique constraint")
			}
			spec.keys = keys
		} else {
			actual := raw.Lookup("partialFilterExpression")
			if actual.Type != bson.TypeEmbeddedDocument {
				return nil, errors.New("original Mongo partial unique filter missing")
			}
			var got, want bson.M
			if err := bson.Unmarshal(actual.Document(), &got); err != nil {
				return nil, err
			}
			expected, err := bson.Marshal(spec.partial)
			if err != nil {
				return nil, err
			}
			if err = bson.Unmarshal(expected, &want); err != nil {
				return nil, err
			}
			if !reflect.DeepEqual(got, want) {
				return nil, errors.New("actual Mongo partial unique filter differs from original fixed resource")
			}
		}
		if !graphMongoIndexKeys(key.Document(), spec.keys) {
			return nil, errors.New("actual Mongo unique key differs from original fixed resource")
		}
		out = append(out, spec)
	}
	if !primary {
		return nil, errors.New("actual Mongo primary index missing")
	}
	return out, nil
}
func graphMongoIndexKeys(raw bson.Raw, keys []string) bool {
	elements, err := raw.Elements()
	if err != nil || len(elements) != len(keys) {
		return false
	}
	for i, element := range elements {
		if element.Key() != keys[i] || element.Value().Type != bson.TypeInt32 || element.Value().Int32() != 1 {
			return false
		}
	}
	return true
}
func graphMongoSimpleCollation(v bson.RawValue) bool {
	if v.Type != bson.TypeEmbeddedDocument {
		return false
	}
	elements, err := v.Document().Elements()
	return err == nil && len(elements) == 1 && elements[0].Key() == "locale" && elements[0].Value().Type == bson.TypeString && elements[0].Value().StringValue() == "simple"
}

func graphMongoUniqueValues(raw bson.Raw, spec graphMongoUniqueSpec) ([][]bson.RawValue, error) {
	if spec.mode == "durable_event" {
		parent := raw.Lookup("durable_acceptance")
		if parent.Type == 0 || parent.Type == bson.TypeNull {
			return nil, nil
		}
		if parent.Type != bson.TypeEmbeddedDocument {
			return nil, errors.New("uninterpretable original durable acceptance unique shape")
		}
		v := parent.Document().Lookup("event_id")
		if v.Type == bson.TypeArray || v.Type == bson.TypeEmbeddedDocument {
			return nil, errors.New("uninterpretable original durable event unique value")
		}
		if v.Type != bson.TypeString {
			return nil, nil
		}
		return [][]bson.RawValue{{v}}, nil // $type:string includes the empty string.
	}
	if spec.mode == "positive_start" {
		parent := raw.Lookup("start_context")
		if parent.Type == 0 || parent.Type == bson.TypeNull {
			return nil, nil
		}
		if parent.Type != bson.TypeEmbeddedDocument {
			return nil, errors.New("uninterpretable original answering-start unique shape")
		}
		v := parent.Document().Lookup("id")
		if v.Type == 0 || v.Type == bson.TypeNull {
			return nil, nil
		}
		switch v.Type {
		case bson.TypeInt32:
			if v.Int32() <= 0 {
				return nil, nil
			}
		case bson.TypeInt64:
			if v.Int64() <= 0 {
				return nil, nil
			}
		case bson.TypeArray, bson.TypeDouble, bson.TypeDecimal128, bson.TypeEmbeddedDocument:
			return nil, errors.New("uninterpretable original answering-start unique value")
		default:
			return nil, nil // Not a numeric $gt:0 index entry.
		}
		return [][]bson.RawValue{{v}}, nil
	}
	if spec.mode == "nonempty_event" {
		v := raw.Lookup(spec.keys[0])
		if v.Type == bson.TypeArray {
			return nil, errors.New("uninterpretable original event unique array")
		}
		if v.Type != bson.TypeString || v.StringValue() == "" {
			return nil, nil
		}
		return [][]bson.RawValue{{v}}, nil
	}
	if spec.mode == "event_entries" {
		parent := raw.Lookup(strings.Split(spec.keys[0], ".")[0])
		if parent.Type == 0 || parent.Type == bson.TypeNull {
			return nil, nil
		}
		if parent.Type != bson.TypeEmbeddedDocument {
			return nil, errors.New("uninterpretable original historical unique shape")
		}
		entries := parent.Document().Lookup("entries")
		if entries.Type == 0 || entries.Type == bson.TypeNull {
			return nil, nil
		}
		if entries.Type != bson.TypeArray {
			return nil, errors.New("uninterpretable original historical unique entries")
		}
		values, err := entries.Array().Values()
		if err != nil {
			return nil, err
		}
		out, unsupported := [][]bson.RawValue{}, false
		for _, entry := range values {
			if entry.Type != bson.TypeEmbeddedDocument {
				return nil, errors.New("uninterpretable original historical unique entry")
			}
			v := entry.Document().Lookup("event_id")
			if v.Type == bson.TypeArray {
				return nil, errors.New("uninterpretable original historical unique event array")
			}
			if v.Type == bson.TypeString {
				out = append(out, []bson.RawValue{v})
			} else {
				unsupported = true
			}
		}
		if len(out) == 0 {
			return nil, nil
		} // The document does not match $type:string.
		if unsupported {
			return nil, errors.New("mixed original historical unique entry types require explicit ownership resolution")
		}
		return out, nil
	}
	values := make([]bson.RawValue, len(spec.keys))
	for i, key := range spec.keys {
		v := raw.Lookup(key)
		if v.Type == 0 {
			v = bson.RawValue{Type: bson.TypeNull}
		}
		switch v.Type {
		case bson.TypeInt32, bson.TypeInt64, bson.TypeDouble, bson.TypeDecimal128, bson.TypeString, bson.TypeBinary, bson.TypeObjectID, bson.TypeBoolean, bson.TypeDateTime, bson.TypeTimestamp, bson.TypeNull:
		default:
			return nil, errors.New("uninterpretable original Mongo unique scalar")
		}
		values[i] = v
	}
	return [][]bson.RawValue{values}, nil
}

func graphMongoUniqueFilter(records []record, spec graphMongoUniqueSpec) (bson.D, error) {
	clauses, seen, size := bson.A{}, map[string]bool{}, 4096
	for _, r := range records {
		values, err := graphMongoUniqueValues(bson.Raw(r.BSON), spec)
		if err != nil {
			return nil, err
		}
		for _, tuple := range values {
			condition := bson.D{}
			for i, key := range spec.keys {
				condition = append(condition, bson.E{Key: key, Value: bson.D{{Key: "$eq", Value: tuple[i]}}})
			}
			if len(spec.partial) != 0 {
				condition = bson.D{{Key: "$and", Value: bson.A{condition, spec.partial}}}
			}
			encoded, err := bson.Marshal(condition)
			if err != nil {
				return nil, err
			}
			if !seen[string(encoded)] {
				seen[string(encoded)] = true
				size += len(encoded) + 32
				if size > graphMongoCommandBytes {
					return nil, fmt.Errorf("%w: Mongo unique collision probe", errGraphBatchBudget)
				}
				clauses = append(clauses, condition)
			}
		}
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	return bson.D{{Key: "$or", Value: clauses}}, nil
}
func (s *graphStore) graphMongoUniquePreflight(ctx context.Context, records []record, definition []byte) error {
	specs, err := graphMongoUniqueIndexes(definition, records[0].Object.Name, s.live.mongoDB)
	if err != nil {
		return err
	}
	expected := map[string]record{}
	for _, r := range records {
		expected[objectID(r.Object)] = r
	}
	for _, spec := range specs {
		filter, err := graphMongoUniqueFilter(records, spec)
		if err != nil {
			return err
		}
		if len(filter) == 0 {
			continue
		}
		cur, err := s.live.mongo.Database(s.live.mongoDB).Collection(records[0].Object.Name).Find(ctx, filter, options.Find().SetBatchSize(500).SetLimit(501).SetCollation(&options.Collation{Locale: "simple"}))
		if err != nil {
			return err
		}
		count := 0
		for cur.Next(ctx) {
			count++
			r, err := graphMongoObject(records[0].Object.Name, cur.Current, definition)
			if err != nil {
				return errors.Join(err, cur.Close(ctx))
			}
			original, ok := expected[objectID(r.Object)]
			if !ok || !sameRecord(r, original) || count > len(records) {
				return errors.Join(errors.New("restore Mongo unique constraint conflicts with a foreign or changed original"), cur.Close(ctx))
			}
		}
		if err = errors.Join(cur.Err(), cur.Close(ctx)); err != nil {
			return err
		}
	}
	return nil
}

// Information_schema silently filters objects without sufficient privileges.
// Direct grants on the actual current schema (or globally) prove this
// schema's visibility. Cross-schema dependency inventory remains the executor
// preflight's responsibility. Role expansion and partial revokes are not
// inferred; original grant text remains bounded RAM data and is never emitted.
func graphBatchSQLVisibility(ctx context.Context, q sqlReader) error {
	var database string
	if err := q.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		return err
	}
	if database == "" {
		return errors.New("actual SQL schema is unknown")
	}
	schemaScope := "`" + strings.ReplaceAll(database, "`", "``") + "`.*"
	rows, err := q.QueryContext(ctx, "SHOW GRANTS FOR CURRENT_USER")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	grants, count, size := map[string]bool{}, 0, 0
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		count++
		size += len(raw)
		text := strings.TrimSpace(raw)
		if count > 100 || size > 64*1024 || !strings.HasPrefix(text, "GRANT ") {
			return errors.New("target SQL dependency metadata visibility unproven")
		}
		privileges, scopeAndUser, found := strings.Cut(text[6:], " ON ")
		if !found {
			continue
		}
		scope, _, found := strings.Cut(scopeAndUser, " TO ")
		if found && (scope == "*.*" || scope == schemaScope) {
			for _, privilege := range strings.Split(privileges, ",") {
				grants[strings.ToUpper(strings.TrimSpace(privilege))] = true
			}
		}
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if !grants["ALL PRIVILEGES"] && (!grants["SELECT"] || !grants["TRIGGER"]) {
		return errors.New("target SQL dependency metadata visibility unproven")
	}
	return nil
}

func graphBatchPreimages(records []record, current []graphReadResult, restore bool) ([]record, error) {
	if len(records) != len(current) {
		return nil, errors.New("graph batch read cardinality changed")
	}
	pending := []record{}
	for i, r := range records {
		a := current[i]
		if !bytes.Equal(a.Record.Definition, r.Definition) {
			return nil, errors.New("graph namespace definition changed")
		}
		if !a.Exists {
			if !restore {
				return nil, errors.New("delete original graph row is missing")
			}
			pending = append(pending, r)
		} else if !sameRecord(a.Record, r) {
			return nil, errors.New("original graph preimage conflicts with current physical key")
		} else if !restore {
			pending = append(pending, r)
		}
	}
	return pending, nil
}

func (s *graphStore) mutateExactBatch(ctx context.Context, records []record, restore bool) (err error) {
	if err = s.preflightExactBatch(ctx, records, restore); err != nil {
		return err
	}
	objects := make([]object, len(records))
	for i, r := range records {
		objects[i] = r.Object
	}
	if objects[0].Store == "mysql" {
		return s.mutateGraphSQLBatch(ctx, objects, records, restore)
	}
	definition := records[0].Definition // verified outside the data transaction
	session, err := s.live.mongo.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	return mongo.WithSession(ctx, session, func(sc mongo.SessionContext) (err error) {
		if err = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority())); err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				err = errors.Join(err, session.AbortTransaction(sc))
			}
		}()
		current, err := s.readGraphMongoBatch(sc, objects, definition)
		if err != nil {
			return err
		}
		pending, err := graphBatchPreimages(records, current, restore)
		if err != nil {
			return err
		}
		if restore {
			if err = s.graphMongoUniquePreflight(sc, records, definition); err != nil {
				return err
			}
		}
		if len(pending) > 0 {
			coll := s.live.mongo.Database(s.live.mongoDB).Collection(objects[0].Name)
			if restore {
				docs := make([]any, len(pending))
				for i, r := range pending {
					docs[i] = bson.Raw(r.BSON)
				}
				ack, err := coll.InsertMany(sc, docs, options.InsertMany().SetOrdered(true))
				if err != nil || len(ack.InsertedIDs) != len(pending) {
					return errors.Join(err, errors.New("graph Mongo insert acknowledgement count changed"))
				}
			} else {
				filter, err := graphMongoBatchFilter(objects)
				if err != nil {
					return err
				}
				ack, err := coll.DeleteMany(sc, filter, options.Delete().SetCollation(&options.Collation{Locale: "simple"}))
				if err != nil || ack.DeletedCount != int64(len(pending)) {
					return errors.Join(err, errors.New("graph Mongo delete acknowledgement count changed"))
				}
			}
		}
		current, err = s.readGraphMongoBatch(sc, objects, definition)
		if err != nil {
			return err
		}
		if err = graphBatchPostimages(records, current, restore); err != nil {
			return err
		}
		err = session.CommitTransaction(sc) // No WithTransaction/business retry.
		committed = err == nil
		return err
	})
}

func graphBatchPostimages(records []record, current []graphReadResult, restore bool) error {
	if len(records) != len(current) {
		return errors.New("graph postimage count changed")
	}
	for i, r := range records {
		if restore && (!current[i].Exists || !sameRecord(r, current[i].Record)) || !restore && current[i].Exists {
			return fmt.Errorf("graph write postimage mismatch for %s/%s", r.Object.Store, r.Object.Name)
		}
	}
	return nil
}

func (s *graphStore) mutateGraphSQLBatch(ctx context.Context, objects []object, records []record, restore bool) (err error) {
	tx, err := s.live.beginGraphTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollback := tx.Rollback(); rollback != nil && !errors.Is(rollback, sql.ErrTxDone) {
				err = errors.Join(err, rollback)
			}
		}
	}()
	shape, err := graphBatchSQLShape(ctx, tx, objects[0].Name)
	if err != nil {
		return err
	}
	current, err := graphReadSQLBatch(ctx, tx, objects, shape, true)
	if err != nil {
		return err
	}
	pending, err := graphBatchPreimages(records, current, restore)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		query, args, err := graphSQLWritePlan(ctx, tx, pending, shape, restore)
		if err != nil {
			return err
		}
		ack, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		count, err := ack.RowsAffected()
		if err != nil || count != int64(len(pending)) {
			return errors.Join(err, errors.New("graph SQL write acknowledgement count changed"))
		}
	}
	current, err = graphReadSQLBatch(ctx, tx, objects, shape, true)
	if err != nil {
		return err
	}
	if err = graphBatchPostimages(records, current, restore); err != nil {
		return err
	}
	err = tx.Commit() // Unknown commit is returned, never replayed here.
	committed = err == nil
	return err
}
