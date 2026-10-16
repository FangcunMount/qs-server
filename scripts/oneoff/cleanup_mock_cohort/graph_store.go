package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type graphSQLShape struct {
	keys       []cell
	definition []byte
}
type graphStore struct {
	live             *liveStore
	sqlShapes        map[string]graphSQLShape
	mongoDefinitions map[string][]byte
}

func newGraphStore(s *liveStore) *graphStore {
	return &graphStore{s, map[string]graphSQLShape{}, map[string][]byte{}}
}

var graphSQLTables = map[string]int{
	"testee": 10, "clinician_relation": 20, "testee_store_history": 20, "assessment_entry_intake_log": 20, "answering_start": 20,
	"plan_enrollment": 30, "assessment_task": 40, "assessment": 50, "runtime_checkpoint": 60, "evaluation_outcome": 60, "assessment_score": 70,
	"interpretation_attention_projection": 80, "interpretation_admission_failure": 80,
	"qs_rm_evaluation_request_ref": 80, "qs_rm_replay_items": 90, "qs_rm_replay_requests": 85, "retry_event_hold": 90, "event_delivery_dead_letter": 90, "rm_outbox": 90,
	"task_opened_reminder_batch": 80, "task_opened_reminder_delivery": 90,
	"ai_bridge_requests": 0, "statistics_access_fact": 90, "statistics_assessment_fact": 90, "statistics_plan_fact": 90, "statistics_store_activity_fact": 90,
}
var graphMongoTables = map[string]int{"answersheets": 50, "report_generations": 70, "interpretation_runs": 80, "interpret_report_artifacts": 90, "report_query_catalog": 100, "rm_outbox": 110, "qs_rm_replay_requests": 110}

func graphSQLKind(dataType, columnType string) string {
	switch dataType {
	case "tinyint", "smallint", "mediumint", "int", "bigint":
		if strings.Contains(columnType, "unsigned") {
			return "uint64"
		}
		return "int64"
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return "text"
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return "bytes"
	}
	return ""
}
func (s *graphStore) shape(ctx context.Context, name string) (graphSQLShape, error) {
	if v, ok := s.sqlShapes[name]; ok {
		return v, nil
	}
	var v graphSQLShape
	if _, ok := graphSQLTables[name]; !ok {
		return v, errors.New("table outside fixed cohort graph")
	}
	rows, e := s.live.sql.QueryContext(ctx, "SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=k.TABLE_SCHEMA AND c.TABLE_NAME=k.TABLE_NAME AND c.COLUMN_NAME=k.COLUMN_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.CONSTRAINT_NAME='PRIMARY' ORDER BY k.COLUMN_NAME", name)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var key, typ, col string
		if e = rows.Scan(&key, &typ, &col); e != nil {
			_ = rows.Close()
			return v, e
		}
		kind := graphSQLKind(typ, col)
		if !keyIdentifier.MatchString(key) || kind == "" {
			_ = rows.Close()
			return v, errors.New("unsupported actual graph primary key")
		}
		v.keys = append(v.keys, cell{Name: key, Type: kind})
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil {
		return v, re
	}
	if ce != nil {
		return v, ce
	}
	if len(v.keys) == 0 {
		return v, errors.New("actual primary key required for graph namespace")
	}
	var table, ddl, zone, charset, mode string
	if e = s.live.sql.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&table, &ddl); e != nil {
		return v, e
	}
	if e = s.live.sql.QueryRowContext(ctx, "SELECT @@session.time_zone,@@session.character_set_results,@@session.sql_mode").Scan(&zone, &charset, &mode); e != nil {
		return v, e
	}
	v.definition = encode([]string{ddl, zone, charset, mode})
	s.sqlShapes[name] = v
	return v, nil
}
func scanGraphSQLRow(rows *sql.Rows, names []string, types []*sql.ColumnType, shape graphSQLShape, name string) (record, error) {
	r := record{Object: object{Store: "mysql", Name: name, Key: []cell{}}, Columns: []cell{}, Definition: shape.definition}
	values := make([]sql.RawBytes, len(names))
	dest := make([]any, len(values))
	for i := range values {
		dest[i] = &values[i]
	}
	if e := rows.Scan(dest...); e != nil {
		return r, e
	}
	for i, v := range values {
		c := cell{Name: names[i], Type: types[i].DatabaseTypeName(), Null: v == nil}
		if v != nil {
			c.Raw = append([]byte{}, v...)
		}
		r.Columns = append(r.Columns, c)
	}
	for _, k := range shape.keys {
		found := false
		for _, c := range r.Columns {
			if c.Name == k.Name {
				if c.Null {
					return r, errors.New("null physical key")
				}
				k.Raw = append([]byte{}, c.Raw...)
				r.Object.Key = append(r.Object.Key, k)
				found = true
				break
			}
		}
		if !found {
			return r, errors.New("actual key missing from graph row")
		}
	}
	if e := validateObject(r.Object); e != nil {
		return r, e
	}
	if _, _, e := keyWhere(r.Object); e != nil {
		return r, e
	}
	if len(encode(r)) > 32*1024*1024 {
		return r, errors.New("single graph row exceeds 32MiB")
	}
	return r, nil
}
func graphKeyset(keys []cell) (string, []any) {
	// Real PK order/type comes from metadata. No caller-provided SQL identifier or
	// arbitrary query reaches this builder; text ordering follows the PK's own
	// collation, while physical identity is still the returned original bytes.
	names := []string{}
	slots := []string{}
	args := []any{}
	for _, k := range keys {
		names = append(names, "x.`"+k.Name+"`")
		slots = append(slots, "?")
		args = append(args, k.Raw)
	}
	return "(" + strings.Join(names, ",") + ") > (" + strings.Join(slots, ",") + ")", args
}
func (s *graphStore) sqlPages(ctx context.Context, name, where string, args []any, emit func(record) error) error {
	shape, e := s.shape(ctx, name)
	if e != nil {
		return e
	}
	order := []string{}
	for _, k := range shape.keys {
		order = append(order, "x.`"+k.Name+"`")
	}
	var after []cell
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		condition := where
		argv := append([]any{}, args...)
		if len(after) > 0 {
			q, a := graphKeyset(after)
			condition = "(" + where + ") AND " + q
			argv = append(argv, a...)
		}
		rows, e := s.live.sql.QueryContext(ctx, "SELECT x.* FROM `"+name+"` x WHERE "+condition+" ORDER BY "+strings.Join(order, ",")+" LIMIT 1000", argv...)
		if e != nil {
			return e
		}
		names, e := rows.Columns()
		if e != nil {
			_ = rows.Close()
			return e
		}
		types, e := rows.ColumnTypes()
		if e != nil {
			_ = rows.Close()
			return e
		}
		n := 0
		page := []record{}
		pageBytes := 0
		for rows.Next() {
			r, e := scanGraphSQLRow(rows, names, types, shape, name)
			if e != nil {
				_ = rows.Close()
				return e
			}
			n++
			after = r.Object.Key
			pageBytes += len(encode(r))
			if pageBytes > 64*1024*1024 {
				_ = rows.Close()
				return errors.New("SQL discovery page exceeds fixed 64MiB; use smaller subset")
			}
			page = append(page, r)
		}
		re, ce := rows.Err(), rows.Close()
		if re != nil {
			return re
		}
		if ce != nil {
			return ce
		}
		for _, r := range page {
			if e = emit(r); e != nil {
				return e
			}
		}
		if n < 1000 {
			return nil
		}
	}
}
func placeholders(ids []string) (string, []any) {
	p := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		p[i] = "?"
		args[i] = id
	}
	return strings.Join(p, ","), args
}
func (s *graphStore) mongoDefinition(ctx context.Context, name string) ([]byte, error) {
	if d, ok := s.mongoDefinitions[name]; ok {
		return d, nil
	}
	if _, ok := graphMongoTables[name]; !ok {
		return nil, errors.New("collection outside fixed cohort graph")
	}
	coll := s.live.mongo.Database(s.live.mongoDB).Collection(name)
	indexes, e := coll.Indexes().List(ctx)
	if e != nil {
		return nil, e
	}
	definitions := []bson.Raw{}
	for indexes.Next(ctx) {
		definitions = append(definitions, append(bson.Raw{}, indexes.Current...))
	}
	re, ce := indexes.Err(), indexes.Close(ctx)
	if re != nil {
		return nil, re
	}
	if ce != nil {
		return nil, ce
	}
	meta, e := s.live.mongo.Database(s.live.mongoDB).ListCollections(ctx, bson.D{{Key: "name", Value: name}})
	if e != nil {
		return nil, e
	}
	n := 0
	var definition bson.Raw
	for meta.Next(ctx) {
		n++
		definition = append(bson.Raw{}, meta.Current...)
	}
	re, ce = meta.Err(), meta.Close(ctx)
	if re != nil {
		return nil, re
	}
	if ce != nil {
		return nil, ce
	}
	if n != 1 {
		return nil, errors.New("graph namespace/UUID must exist")
	}
	d := encode(struct {
		Collection bson.Raw
		Indexes    []bson.Raw
	}{definition, definitions})
	s.mongoDefinitions[name] = d
	return d, nil
}
func graphMongoObject(name string, raw bson.Raw, definition []byte) (record, error) {
	id, e := raw.LookupErr("_id")
	if e != nil {
		return record{}, e
	}
	o := object{Store: "mongo", Name: name, Key: []cell{{Name: "_id", Type: fmt.Sprintf("bson:%02x", byte(id.Type)), Raw: append([]byte{}, id.Value...)}}}
	if e = validateObject(o); e != nil {
		return record{}, e
	}
	if _, e = mongoKey(o); e != nil {
		return record{}, e
	}
	r := record{Object: o, BSON: append([]byte{}, raw...), Definition: definition}
	if len(encode(r)) > 32*1024*1024 {
		return r, errors.New("single graph document exceeds 32MiB")
	}
	return r, nil
}
func (s *graphStore) mongoPages(ctx context.Context, name string, filter bson.D, emit func(record) error) error {
	definition, e := s.mongoDefinition(ctx, name)
	if e != nil {
		return e
	}
	cur, e := s.live.mongo.Database(s.live.mongoDB).Collection(name).Find(ctx, filter, options.Find().SetBatchSize(500).SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		return e
	}
	defer func() { _ = cur.Close(ctx) }()
	// Server batches are <=500; documents are processed one at a time. Exact raw
	// _id types are kept, and no full result slice is materialized in RAM.
	for cur.Next(ctx) {
		r, e := graphMongoObject(name, cur.Current, definition)
		if e != nil {
			return e
		}
		if e = emit(r); e != nil {
			return e
		}
	}
	if e = cur.Err(); e != nil {
		return e
	}
	return cur.Close(ctx)
}
func graphSQLValue(r record, name string) (string, bool) {
	for _, c := range r.Columns {
		if c.Name == name {
			return string(c.Raw), !c.Null
		}
	}
	return "", false
}
func graphDecimal(v string) bool {
	n, e := strconv.ParseUint(v, 10, 64)
	return e == nil && n > 0 && n <= 1<<63-1 && strconv.FormatUint(n, 10) == v
}
func graphMongoID(raw bson.Raw, name string, optional bool) (string, error) {
	v, e := raw.LookupErr(name)
	if e != nil {
		if optional {
			return "", nil
		}
		return "", errors.New("required BSON identity missing")
	}
	if v.Type != bson.TypeInt64 {
		return "", errors.New("BSON business identity must retain actual int64 type")
	}
	n := v.Int64()
	if n == 0 && optional {
		return "", nil
	}
	if n <= 0 {
		return "", errors.New("BSON business identity must be positive")
	}
	return strconv.FormatInt(n, 10), nil
}
func graphMongoIDs(ids []string) bson.A {
	out := make(bson.A, len(ids))
	for i, id := range ids {
		n, _ := strconv.ParseInt(id, 10, 64)
		out[i] = n
	}
	return out
}
func graphMongoString(raw bson.Raw, name string) string {
	v, e := raw.LookupErr(name)
	if e != nil || v.Type != bson.TypeString {
		return ""
	}
	return v.StringValue()
}
func (s *graphStore) verifyDefinitions(ctx context.Context) (map[string]string, error) {
	hashes := map[string]string{}
	oldSQL := s.sqlShapes
	s.sqlShapes = map[string]graphSQLShape{}
	for name, want := range oldSQL {
		actual, e := s.shape(ctx, name)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(actual.definition, want.definition) {
			return nil, errors.New("SQL namespace changed during readonly full graph discovery")
		}
		hashes["mysql/"+name] = digest(actual.definition)
	}
	oldMongo := s.mongoDefinitions
	s.mongoDefinitions = map[string][]byte{}
	for name, want := range oldMongo {
		actual, e := s.mongoDefinition(ctx, name)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(actual, want) {
			return nil, errors.New("mongo namespace/UUID/index changed during readonly full graph discovery")
		}
		hashes["mongo/"+name] = digest(actual)
	}
	return hashes, nil
}
