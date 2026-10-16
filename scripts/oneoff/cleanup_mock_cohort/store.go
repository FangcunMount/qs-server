package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

// This first executable stage is deliberately limited to independently keyed
// leaves. Actor/Plan roots and message retirement need their semantic reference
// graph, not an inferred foreign-key or equal-number join, before being enabled.
var writableLeaves = map[string]bool{
	"mysql/assessment_score":                    true,
	"mysql/interpretation_attention_projection": true,
	"mongo/report_query_catalog":                true,
}

var readableSQL = map[string]bool{
	"testee": true, "operators": true, "clinician": true, "clinician_relation": true,
	"assessment": true, "evaluation_outcome": true, "assessment_score": true,
	"interpretation_attention_projection": true, "interpretation_admission_failure": true,
	"assessment_plan": true, "plan_enrollment": true, "assessment_task": true,
	"assessment_entry": true, "assessment_entry_intake_log": true, "assessment_entry_resolve_log": true,
	"answering_start": true, "testee_store_history": true,
	"statistics_access_fact": true, "statistics_assessment_fact": true, "statistics_plan_fact": true, "statistics_store_activity_fact": true,
}
var readableMongo = map[string]bool{"answersheets": true, "report_generations": true, "interpretation_runs": true, "interpret_report_artifacts": true, "report_query_catalog": true}

type dataStore interface {
	identity(context.Context, binding) error
	read(context.Context, object) (record, bool, error)
	mutate(context.Context, []record, bool) error
}

type liveStore struct {
	sql     *sql.Conn
	mongo   *mongo.Client
	mongoDB string
}

func (s *liveStore) identity(ctx context.Context, w binding) error {
	var uuid, name string
	if e := s.sql.QueryRowContext(ctx, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &name); e != nil {
		return e
	}
	if uuid != w.SQLUUID || name != w.SQLDatabase {
		return errors.New("SQL identity changed")
	}
	rows, e := s.sql.QueryContext(ctx, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		return e
	}
	n := 0
	var version int64
	var dirty bool
	for rows.Next() {
		n++
		if e = rows.Scan(&version, &dirty); e != nil {
			_ = rows.Close()
			return e
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil {
		return re
	}
	if ce != nil {
		return ce
	}
	if n != 1 || version != 101 || dirty {
		return errors.New("clean SQL101 required")
	}
	var hello struct {
		Set     string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
		Primary bool     `bson:"isWritablePrimary"`
		Msg     string   `bson:"msg"`
	}
	if e = s.mongo.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); e != nil {
		return e
	}
	if hello.Set == "" || !hello.Primary || hello.Msg != "" || len(hello.Hosts) == 0 {
		return errors.New("known Mongo primary replica set required")
	}
	sort.Strings(hello.Hosts)
	for i, h := range hello.Hosts {
		if h == "" || i > 0 && h == hello.Hosts[i-1] {
			return errors.New("invalid replica host set")
		}
	}
	identity := digest(encode([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.Set}, {Key: "hosts", Value: hello.Hosts}}, s.mongoDB}))
	if s.mongoDB != w.MongoDatabase || identity != w.MongoIdentity {
		return errors.New("mongo identity changed")
	}
	cur, e := s.mongo.Database(s.mongoDB).Collection("schema_migrations").Find(ctx, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		return e
	}
	n = 0
	for cur.Next(ctx) {
		n++
		var row struct {
			Version int64 `bson:"version"`
			Dirty   *bool `bson:"dirty"`
		}
		if e = cur.Decode(&row); e != nil {
			_ = cur.Close(ctx)
			return e
		}
		if row.Version != 39 || row.Dirty == nil || *row.Dirty {
			_ = cur.Close(ctx)
			return errors.New("clean Mongo39 required")
		}
	}
	re, ce = cur.Err(), cur.Close(ctx)
	if re != nil {
		return re
	}
	if ce != nil {
		return ce
	}
	if n != 1 {
		return errors.New("one Mongo migration ledger required")
	}
	return nil
}

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func keyWhere(o object) (string, []any, error) {
	parts := []string{}
	args := []any{}
	for _, k := range o.Key {
		switch k.Type {
		case "uint64":
			v, e := strconv.ParseUint(string(k.Raw), 10, 64)
			if e != nil || strconv.FormatUint(v, 10) != string(k.Raw) {
				return "", nil, errors.New("noncanonical unsigned key")
			}
		case "int64":
			v, e := strconv.ParseInt(string(k.Raw), 10, 64)
			if e != nil || strconv.FormatInt(v, 10) != string(k.Raw) {
				return "", nil, errors.New("noncanonical signed key")
			}
		case "text", "bytes":
		default:
			return "", nil, errors.New("unsupported SQL key type")
		}
		clause := "`" + k.Name + "` = ?"
		if k.Type == "text" || k.Type == "bytes" {
			clause = "BINARY `" + k.Name + "` = BINARY ?"
		}
		parts = append(parts, clause)
		args = append(args, k.Raw)
	}
	return strings.Join(parts, " AND "), args, nil
}

func readSQL(ctx context.Context, q sqlReader, o object, lock bool) (record, bool, error) {
	var out record
	out.Object = o
	if !readableSQL[o.Name] {
		return out, false, errors.New("SQL object not in fixed read catalog")
	}
	rows, e := q.QueryContext(ctx, "SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=k.TABLE_SCHEMA AND c.TABLE_NAME=k.TABLE_NAME AND c.COLUMN_NAME=k.COLUMN_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.CONSTRAINT_NAME='PRIMARY' ORDER BY k.COLUMN_NAME", o.Name)
	if e != nil {
		return out, false, e
	}
	keys := []string{}
	for rows.Next() {
		var k, dataType, columnType string
		if e = rows.Scan(&k, &dataType, &columnType); e != nil {
			_ = rows.Close()
			return out, false, e
		}
		keys = append(keys, k)
		index := len(keys) - 1
		if index >= len(o.Key) || o.Key[index].Name != k {
			_ = rows.Close()
			return out, false, errors.New("complete actual physical primary key required")
		}
		kind := ""
		switch dataType {
		case "tinyint", "smallint", "mediumint", "int", "bigint":
			kind = "int64"
			if strings.Contains(columnType, "unsigned") {
				kind = "uint64"
			}
		case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
			kind = "text"
		case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
			kind = "bytes"
		}
		if kind == "" || o.Key[index].Type != kind {
			_ = rows.Close()
			return out, false, errors.New("caller key type cannot coerce the actual SQL primary key")
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil {
		return out, false, re
	}
	if ce != nil {
		return out, false, ce
	}
	wanted := []string{}
	for _, k := range o.Key {
		wanted = append(wanted, k.Name)
	}
	if !reflect.DeepEqual(keys, wanted) {
		return out, false, errors.New("entire actual physical primary key is required")
	}
	var table, ddl, zone, charset, mode string
	if e = q.QueryRowContext(ctx, "SHOW CREATE TABLE `"+o.Name+"`").Scan(&table, &ddl); e != nil {
		return out, false, e
	}
	if e = q.QueryRowContext(ctx, "SELECT @@session.time_zone,@@session.character_set_results,@@session.sql_mode").Scan(&zone, &charset, &mode); e != nil {
		return out, false, e
	}
	out.Definition = encode([]string{ddl, zone, charset, mode})
	where, args, e := keyWhere(o)
	if e != nil {
		return out, false, e
	}
	query := "SELECT * FROM `" + o.Name + "` WHERE " + where
	if lock {
		query += " FOR UPDATE"
	}
	rows, e = q.QueryContext(ctx, query, args...)
	if e != nil {
		return out, false, e
	}
	names, e := rows.Columns()
	if e != nil {
		_ = rows.Close()
		return out, false, e
	}
	types, e := rows.ColumnTypes()
	if e != nil {
		_ = rows.Close()
		return out, false, e
	}
	n := 0
	for rows.Next() {
		n++
		values := make([]sql.RawBytes, len(names))
		dest := make([]any, len(names))
		for i := range values {
			dest[i] = &values[i]
		}
		if e = rows.Scan(dest...); e != nil {
			_ = rows.Close()
			return out, false, e
		}
		for i, v := range values {
			c := cell{Name: names[i], Type: types[i].DatabaseTypeName(), Null: v == nil}
			if v != nil {
				c.Raw = append([]byte{}, v...)
			}
			out.Columns = append(out.Columns, c)
		}
		for _, key := range o.Key {
			matched := false
			for i, name := range names {
				if name == key.Name {
					matched = bytes.Equal(values[i], key.Raw)
					break
				}
			}
			if !matched {
				_ = rows.Close()
				return out, false, errors.New("selected SQL key differs from exact original key bytes")
			}
		}
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil {
		return out, false, re
	}
	if ce != nil {
		return out, false, ce
	}
	if n > 1 {
		return out, false, errors.New("physical primary key selected multiple rows")
	}
	return out, n == 1, nil
}

func mongoKey(o object) (bson.D, error) {
	if len(o.Key) != 1 || o.Key[0].Name != "_id" {
		return nil, errors.New("physical Mongo _id required")
	}
	k := o.Key[0]
	types := map[string]bsontype.Type{"bson:07": bson.TypeObjectID, "bson:02": bson.TypeString, "bson:12": bson.TypeInt64, "bson:10": bson.TypeInt32, "bson:05": bson.TypeBinary}
	t, ok := types[k.Type]
	if !ok {
		return nil, errors.New("unsupported BSON identity type")
	}
	v := bson.RawValue{Type: t, Value: k.Raw}
	if e := v.Validate(); e != nil {
		return nil, e
	}
	return bson.D{{Key: "_id", Value: v}}, nil
}

func (s *liveStore) readMongo(ctx context.Context, o object) (record, bool, error) {
	r := record{Object: o}
	if !readableMongo[o.Name] {
		return r, false, errors.New("mongo object not in fixed read catalog")
	}
	_, e := mongoKey(o)
	if e != nil {
		return r, false, e
	}
	coll := s.mongo.Database(s.mongoDB).Collection(o.Name)
	indexes, e := coll.Indexes().List(ctx)
	if e != nil {
		return r, false, e
	}
	definitions := []bson.Raw{}
	for indexes.Next(ctx) {
		definitions = append(definitions, append(bson.Raw{}, indexes.Current...))
	}
	re, ce := indexes.Err(), indexes.Close(ctx)
	if re != nil {
		return r, false, re
	}
	if ce != nil {
		return r, false, ce
	}
	meta, e := s.mongo.Database(s.mongoDB).ListCollections(ctx, bson.D{{Key: "name", Value: o.Name}})
	if e != nil {
		return r, false, e
	}
	n := 0
	var definition bson.Raw
	for meta.Next(ctx) {
		n++
		definition = append(bson.Raw{}, meta.Current...)
	}
	re, ce = meta.Err(), meta.Close(ctx)
	if re != nil {
		return r, false, re
	}
	if ce != nil {
		return r, false, ce
	}
	if n != 1 {
		return r, false, errors.New("mongo namespace/UUID must exist")
	}
	r.Definition = encode(struct {
		Collection bson.Raw
		Indexes    []bson.Raw
	}{definition, definitions})
	return s.readMongoBody(ctx, o, r.Definition)
}

// Catalog commands are performed outside the data transaction. MongoDB forbids
// listCollections/listIndexes inside a multi-document transaction.
func (s *liveStore) readMongoBody(ctx context.Context, o object, definition []byte) (record, bool, error) {
	r := record{Object: o, Definition: append([]byte{}, definition...)}
	key, e := mongoKey(o)
	if e != nil {
		return r, false, e
	}
	raw, e := s.mongo.Database(s.mongoDB).Collection(o.Name).FindOne(ctx, key).Raw()
	if errors.Is(e, mongo.ErrNoDocuments) {
		return r, false, nil
	}
	if e != nil {
		return r, false, e
	}
	if e = exactMongoKey(raw, o); e != nil {
		return r, false, e
	}
	r.BSON = append([]byte{}, raw...)
	return r, true, nil
}

// MongoDB numeric and collation equality is broader than BSON identity. A query
// match is never sufficient proof that an int32/string alias owns this object.
func exactMongoKey(raw bson.Raw, o object) error {
	key, e := mongoKey(o)
	if e != nil {
		return e
	}
	want := key[0].Value.(bson.RawValue)
	elements, e := raw.Elements()
	if e != nil {
		return e
	}
	count := 0
	for _, element := range elements {
		if element.Key() == "_id" {
			count++
			got := element.Value()
			if got.Type != want.Type || !bytes.Equal(got.Value, want.Value) {
				return errors.New("mongo query matched a different physical typed _id")
			}
		}
	}
	if count != 1 {
		return errors.New("exactly one original physical _id required")
	}
	return nil
}

func (s *liveStore) read(ctx context.Context, o object) (record, bool, error) {
	if o.Store == "mysql" {
		return readSQL(ctx, s.sql, o, false)
	}
	return s.readMongo(ctx, o)
}

func sameRecord(a, b record) bool { return fingerprint(a) == fingerprint(b) }

func sqlValue(c cell) (any, error) {
	if c.Null {
		return nil, nil
	}
	if c.Type == "BIT" {
		if len(c.Raw) > 8 {
			return nil, errors.New("unsupported BIT width")
		}
		var v uint64
		for _, b := range c.Raw {
			v = v<<8 | uint64(b)
		}
		return v, nil
	}
	if c.Type == "GEOMETRY" {
		return nil, errors.New("geometry restoration not supported by this fixed catalog")
	}
	return c.Raw, nil
}

func generatedColumns(ctx context.Context, q sqlReader, table string) (map[string]bool, error) {
	rows, e := q.QueryContext(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''", table)
	if e != nil {
		return nil, e
	}
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			_ = rows.Close()
			return nil, e
		}
		out[name] = true
	}
	re, ce := rows.Err(), rows.Close()
	return out, errors.Join(re, ce)
}

func (s *liveStore) mutate(ctx context.Context, records []record, restore bool) error {
	if len(records) == 0 {
		return errors.New("empty mutation")
	}
	store := records[0].Object.Store
	limit := 1000
	if store == "mongo" {
		limit = 500
	}
	if len(records) > limit {
		return errors.New("chunk limit exceeded")
	}
	for _, r := range records {
		if r.Object.Store != store || !writableLeaves[r.Object.Store+"/"+r.Object.Name] {
			return errors.New("semantic reference graph not yet implemented for this root; excluded, never writable")
		}
	}
	if store == "mysql" {
		tx, e := s.sql.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback() }()
		for _, r := range records {
			actual, exists, e := readSQL(ctx, tx, r.Object, true)
			if e != nil {
				return e
			}
			if !bytes.Equal(actual.Definition, r.Definition) {
				return errors.New("SQL schema/session changed")
			}
			if restore {
				if exists {
					if !sameRecord(actual, r) {
						return errors.New("restore conflicts with current original PK")
					}
					continue
				}
				cols := []string{}
				params := []string{}
				args := []any{}
				generated, e := generatedColumns(ctx, tx, r.Object.Name)
				if e != nil {
					return e
				}
				for _, c := range r.Columns {
					if generated[c.Name] {
						continue
					}
					cols = append(cols, "`"+c.Name+"`")
					params = append(params, "?")
					value, e := sqlValue(c)
					if e != nil {
						return e
					}
					args = append(args, value)
				}
				if _, e = tx.ExecContext(ctx, "INSERT INTO `"+r.Object.Name+"` ("+strings.Join(cols, ",")+") VALUES ("+strings.Join(params, ",")+")", args...); e != nil {
					return e
				}
			} else {
				if !exists || !sameRecord(actual, r) {
					return errors.New("delete preimage changed")
				}
				where, args, e := keyWhere(r.Object)
				if e != nil {
					return e
				}
				result, e := tx.ExecContext(ctx, "DELETE FROM `"+r.Object.Name+"` WHERE "+where, args...)
				if e != nil {
					return e
				}
				n, e := result.RowsAffected()
				if e != nil || n != 1 {
					return errors.New("delete acknowledgement not exactly one")
				}
			}
		}
		return tx.Commit() // Any error is treated as unknown by the durable caller.
	}
	for _, r := range records {
		current, _, e := s.readMongo(ctx, r.Object)
		if e != nil {
			return e
		}
		if !bytes.Equal(current.Definition, r.Definition) {
			return errors.New("mongo UUID/index definitions changed")
		}
	}
	session, e := s.mongo.StartSession()
	if e != nil {
		return e
	}
	defer session.EndSession(ctx)
	return mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		if e := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority())); e != nil {
			return e
		}
		defer func() { _ = session.AbortTransaction(sc) }()
		for _, r := range records {
			actual, exists, e := s.readMongoBody(sc, r.Object, r.Definition)
			if e != nil {
				return e
			}
			if !bytes.Equal(actual.Definition, r.Definition) {
				return errors.New("mongo UUID/index definitions changed")
			}
			key, e := mongoKey(r.Object)
			if e != nil {
				return e
			}
			coll := s.mongo.Database(s.mongoDB).Collection(r.Object.Name)
			if restore {
				if exists {
					if !sameRecord(actual, r) {
						return errors.New("restore BSON conflicts")
					}
					continue
				}
				if _, e = coll.InsertOne(sc, bson.Raw(r.BSON)); e != nil {
					return e
				}
			} else {
				if !exists || !sameRecord(actual, r) {
					return errors.New("delete BSON preimage changed")
				}
				result, e := coll.DeleteOne(sc, key)
				if e != nil {
					return e
				}
				if result.DeletedCount != 1 {
					return errors.New("mongo delete acknowledgement not one")
				}
			}
		}
		return session.CommitTransaction(sc) // No automatic WithTransaction retry.
	})
}

func assertFingerprint(r record, o object) error {
	if o.Fingerprint == "" || o.Fingerprint != fingerprint(r) {
		return fmt.Errorf("preimage changed for %s/%s", o.Store, o.Name)
	}
	return nil
}
