package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type database struct {
	conn    *sql.Conn
	mongo   *mongo.Client
	binding binding
}
type observation struct {
	Binding binding
	Ledger  ledgerRow
	Mongo   []byte
}

func readLedger(ctx context.Context, r sqlReader) ([]ledgerRow, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		return nil, errors.New("sql_ledger_read_failed")
	}
	var out []ledgerRow
	for rows.Next() {
		var row ledgerRow
		if rows.Scan(&row.Version, &row.Dirty) != nil {
			_ = rows.Close()
			return nil, errors.New("sql_ledger_invalid")
		}
		out = append(out, row)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || len(out) != 1 {
		return nil, errors.New("sql_ledger_or_release_invalid")
	}
	return out, nil
}
func requireLedgerShape(ctx context.Context, r sqlReader) error {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='schema_migrations' ORDER BY ORDINAL_POSITION")
	if e != nil {
		return errors.New("complete_ledger_shape_unavailable")
	}
	var got []column
	for rows.Next() {
		var c column
		var nullable string
		if rows.Scan(&c.Name, &c.Type, &nullable) != nil || nullable != "NO" {
			_ = rows.Close()
			return errors.New("complete_ledger_shape_rejected")
		}
		got = append(got, c)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !reflect.DeepEqual(got, []column{{Name: "version", Type: "bigint"}, {Name: "dirty", Type: "tinyint(1)"}}) {
		return errors.New("complete_ledger_shape_or_release_rejected")
	}
	return nil
}
func (d database) observe(ctx context.Context, r sqlReader) (out observation, err error) {
	out.Binding = d.binding
	q, cancel := context.WithTimeout(ctx, queryLimit)
	var uuid, name, zone, charset, mode, server string
	e := r.QueryRowContext(q, "SELECT @@server_uuid,DATABASE(),@@session.time_zone,@@session.character_set_results,@@session.sql_mode,VERSION()").Scan(&uuid, &name, &zone, &charset, &mode, &server)
	cancel()
	if e != nil || !uuidRE.MatchString(uuid) || name != d.binding.SQLName || zone != "+00:00" || charset != "utf8mb4" || mode != sessionMode || server != "8.0.36" {
		return out, errors.New("mysql_identity_or_session_rejected")
	}
	raw, _ := json.Marshal([]any{"mysql_server_selected_v1", uuid, name})
	out.Binding.SQLIdentity = fingerprint(raw)
	ledger, e := readLedger(ctx, r)
	if e != nil {
		return out, e
	}
	out.Ledger = ledger[0]
	q, cancel = context.WithTimeout(ctx, queryLimit)
	var hello struct {
		Set   string   `bson:"setName"`
		Hosts []string `bson:"hosts"`
	}
	e = d.mongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	cancel()
	if e != nil || hello.Set != "rs0" || len(hello.Hosts) == 0 || len(hello.Hosts) > 32 {
		return out, errors.New("mongo_cluster_unknown")
	}
	sort.Strings(hello.Hosts)
	for i, host := range hello.Hosts {
		if host == "" || (i > 0 && host == hello.Hosts[i-1]) {
			return out, errors.New("mongo_cluster_invalid")
		}
	}
	raw, _ = json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.Set}, {Key: "hosts", Value: hello.Hosts}}, d.binding.MongoName})
	out.Binding.MongoIdentity = fingerprint(raw)
	q, cancel = context.WithTimeout(ctx, queryLimit)
	cur, e := d.mongo.Database(d.binding.MongoName).Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2).SetBatchSize(2))
	if e != nil {
		cancel()
		return out, errors.New("mongo_ledger_read_failed")
	}
	count := 0
	for cur.Next(q) {
		out.Mongo = append([]byte(nil), cur.Current...)
		count++
	}
	re, ce := cur.Err(), cur.Close(q)
	cancel()
	if re != nil || ce != nil || cur.ID() != 0 || count != 1 || !cleanMongo40(out.Mongo) {
		return out, errors.New("mongo40_ledger_or_release_rejected")
	}
	q, cancel = context.WithTimeout(ctx, queryLimit)
	cur, e = d.mongo.Database(d.binding.MongoName).ListCollections(q, bson.D{{Key: "name", Value: "schema_migrations"}}, options.ListCollections().SetBatchSize(2))
	if e != nil {
		cancel()
		return out, errors.New("mongo_ledger_uuid_unknown")
	}
	count = 0
	valid := true
	for cur.Next(q) {
		subtype, uuid, ok := cur.Current.Lookup("info", "uuid").BinaryOK()
		if !ok || subtype != 4 || len(uuid) != 16 {
			valid = false
		} else {
			out.Binding.MongoUUID = fingerprint(append([]byte{subtype}, uuid...))
		}
		count++
	}
	re, ce = cur.Err(), cur.Close(q)
	cancel()
	if re != nil || ce != nil || cur.ID() != 0 || count != 1 || !valid || out.Binding != d.binding {
		return out, errors.New("external_database_binding_or_uuid_release_rejected")
	}
	return out, nil
}
func requireTransactionalTables(ctx context.Context, r sqlReader) error {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT TABLE_NAME,TABLE_TYPE,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('testee','operators','operator_retirement_tasks','operator_recovery_archives','schema_migrations') ORDER BY TABLE_NAME")
	if e != nil {
		return errors.New("fixed_table_metadata_unavailable")
	}
	names := []string{}
	for rows.Next() {
		var name, kind, engine string
		if rows.Scan(&name, &kind, &engine) != nil || kind != "BASE TABLE" || engine != "InnoDB" {
			_ = rows.Close()
			return errors.New("fixed_innodb_base_tables_required")
		}
		names = append(names, name)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !reflect.DeepEqual(names, []string{"operator_recovery_archives", "operator_retirement_tasks", "operators", "schema_migrations", "testee"}) {
		return errors.New("fixed_table_scope_or_release_unknown")
	}
	return nil
}
func ddl(ctx context.Context, r sqlReader, table string) (string, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var name, text string
	if r.QueryRowContext(q, "SHOW CREATE TABLE `"+table+"`").Scan(&name, &text) != nil || name != table || text == "" || len(text) > 65536 {
		return "", errors.New("fixed_table_ddl_unavailable")
	}
	return text, nil
}
func columns(ctx context.Context, r sqlReader, i int, original bool) ([]column, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", scope[i].table)
	if e != nil {
		return nil, errors.New("fixed_column_metadata_unavailable")
	}
	var out []column
	present := false
	for rows.Next() {
		var c column
		var nullable string
		if rows.Scan(&c.Name, &c.Type, &nullable) != nil || !nameRE.MatchString(c.Name) || c.Type == "" || len(c.Type) > 255 || (nullable != "YES" && nullable != "NO") {
			_ = rows.Close()
			return nil, errors.New("fixed_column_metadata_invalid")
		}
		c.Nullable = nullable == "YES"
		if c.Name == scope[i].target {
			if present || c.Type != "json" || c.Nullable != (i == 0) {
				_ = rows.Close()
				return nil, errors.New("original_json_column_shape_rejected")
			}
			present = true
		} else {
			out = append(out, c)
		}
		if len(out) > 128 {
			_ = rows.Close()
			return nil, errors.New("column_count_limit")
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || len(out) == 0 || (i < 2 && present != original) {
		return nil, errors.New("fixed_column_presence_or_release_rejected")
	}
	rows, e = r.QueryContext(q, "SELECT COLUMN_NAME,NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX", scope[i].table)
	if e != nil {
		return nil, errors.New("primary_key_metadata_unavailable")
	}
	parts := 0
	valid := true
	for rows.Next() {
		var key string
		var nonUnique int
		if rows.Scan(&key, &nonUnique) != nil || key != scope[i].key || nonUnique != 0 {
			valid = false
		}
		parts++
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil || ce != nil || parts != 1 || !valid {
		return nil, errors.New("fixed_single_primary_key_required")
	}
	for _, c := range out {
		if c.Name == scope[i].key && !c.Nullable && ((!scope[i].binaryKey && c.Type == "bigint unsigned") || (scope[i].binaryKey && c.Type == "varchar(64)")) {
			return out, nil
		}
	}
	return nil, errors.New("fixed_primary_key_type_rejected")
}
func retainedHashSQL(cols []column) string {
	var encoded []string
	for _, c := range cols {
		name := "`" + c.Name + "`"
		prefix := hex.EncodeToString([]byte(c.Name + ":" + c.Type + ":"))
		encoded = append(encoded, fmt.Sprintf("X'%s',CASE WHEN %s IS NULL THEN 'N;' ELSE CONCAT('V',OCTET_LENGTH(CAST(%s AS BINARY)),':',HEX(CAST(%s AS BINARY)),';') END", prefix, name, name, name))
	}
	return "SHA2(CONCAT(" + strings.Join(encoded, ",") + "),256)"
}
func pageSQL(i int, cols []column, original, hasLast bool) string {
	key := "`" + scope[i].key + "`"
	selected, lower, order := "CAST("+key+" AS CHAR)", key+">=0", key
	if scope[i].binaryKey {
		selected, lower, order = "HEX(CAST("+key+" AS BINARY))", "1=1", "CAST("+key+" AS BINARY)"
	}
	if hasLast {
		lower = key + ">CAST(? AS DECIMAL(20,0))"
		if scope[i].binaryKey {
			lower = "CAST(" + key + " AS BINARY)>UNHEX(?)"
		}
	}
	value, length := "NULL", "NULL"
	if i < 2 && original {
		length = "OCTET_LENGTH(CAST(`" + scope[i].target + "` AS BINARY))"
		value = "CASE WHEN " + length + "<=" + strconv.Itoa(maxValueBytes) + " THEN CAST(`" + scope[i].target + "` AS BINARY) ELSE NULL END"
	}
	return "SELECT /*+ MAX_EXECUTION_TIME(5000) */ " + selected + "," + length + "," + value + "," + retainedHashSQL(cols) + " FROM `" + scope[i].table + "` FORCE INDEX(PRIMARY) WHERE " + lower + " ORDER BY " + order + " LIMIT " + strconv.Itoa(pageSize)
}
func scanTable(ctx context.Context, r sqlReader, i int, cols []column, original bool, used *int) (tableProof, error) {
	out := tableProof{Table: scope[i].table, Columns: cols}
	last := ""
	for {
		args := []any{}
		if last != "" {
			args = append(args, last)
		}
		q, cancel := context.WithTimeout(ctx, queryLimit)
		rows, e := r.QueryContext(q, pageSQL(i, cols, original, last != ""), args...)
		if e != nil {
			cancel()
			return out, errors.New("bounded_primary_page_unavailable")
		}
		count := 0
		for rows.Next() {
			var row rowProof
			var raw []byte
			var length sql.NullInt64
			if rows.Scan(&row.Key, &length, &raw, &row.Retained) != nil || !validKey(row.Key, scope[i].binaryKey) || !hashRE.MatchString(row.Retained) || (last != "" && !keyLess(last, row.Key, scope[i].binaryKey)) || len(raw) > maxValueBytes || (length.Valid && (length.Int64 != int64(len(raw)) || raw == nil)) || (!length.Valid && raw != nil) {
				_ = rows.Close()
				cancel()
				return out, errors.New("bounded_row_or_order_invalid")
			}
			if i < 2 && original {
				row.Value = &originalValue{Kind: "sql_null"}
				if raw != nil {
					row.Value.Kind, row.Value.JSON = "json", append([]byte(nil), raw...)
				}
				if !checkValue(row.Value) {
					_ = rows.Close()
					cancel()
					return out, errors.New("original_json_value_invalid")
				}
			}
			*used += len(raw) + len(row.Key) + 256
			if *used > maxMaterialBytes || len(out.Rows) >= maxRows {
				_ = rows.Close()
				cancel()
				return out, errors.New("material_limit_before_mandatory_eof")
			}
			out.Rows = append(out.Rows, row)
			last = row.Key
			count++
		}
		re, ce := rows.Err(), rows.Close()
		cancel()
		if re != nil || ce != nil {
			return out, errors.New("primary_page_release_unproven")
		}
		out.Pages++
		if count == 0 {
			out.EOF = true
			break
		}
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var count uint64
	if r.QueryRowContext(q, "SELECT COUNT(*) FROM `"+out.Table+"`").Scan(&count) != nil || count != uint64(len(out.Rows)) {
		return out, errors.New("primary_eof_count_conservation_rejected")
	}
	return out, nil
}

func (d database) snapshot(ctx context.Context, tool string, version uint64, full bool) (m material, resultErr error) {
	if e := requireTransactionalTables(ctx, d.conn); e != nil {
		return m, e
	}
	before, e := d.observe(ctx, d.conn)
	if e != nil {
		return m, e
	}
	if before.Ledger != (ledgerRow{version, false}) {
		return m, errors.New("clean101_or102_expected_for_action")
	}
	tx, e := d.conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return m, errors.New("readonly_repeatable_read_unavailable")
	}
	done := false
	defer func() {
		if !done {
			if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
				resultErr = errors.Join(resultErr, errors.New("readonly_transaction_release_unproven"))
			}
		}
	}()
	m = material{Format: 1, Kind: "m4_c_actor_column_protection", ToolSource: tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Binding: before.Binding, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, MongoHead: before.Mongo}
	if e := requireLedgerShape(ctx, tx); e != nil {
		return m, e
	}
	m.Ledger, e = readLedger(ctx, tx)
	if e != nil || m.Ledger[0] != before.Ledger {
		return m, errors.New("snapshot_sql_ledger_changed")
	}
	m.LedgerDDL, e = ddl(ctx, tx, "schema_migrations")
	if e != nil {
		return m, e
	}
	used := 0
	for i := range scope {
		cols, e := columns(ctx, tx, i, version == 101)
		if e != nil {
			return m, e
		}
		t := tableProof{Table: scope[i].table, Columns: cols}
		if full {
			t, e = scanTable(ctx, tx, i, cols, version == 101, &used)
			if e != nil {
				return m, e
			}
		}
		t.DDL, e = ddl(ctx, tx, scope[i].table)
		if e != nil {
			return m, e
		}
		t.RetainedDDL, e = retainedDDL(t.DDL, i, version == 101 && i < 2)
		if e != nil {
			return m, e
		}
		m.Tables[i] = t
	}
	if tx.Commit() != nil {
		return m, errors.New("readonly_transaction_commit_unproven")
	}
	done = true
	after, e := d.observe(ctx, d.conn)
	if e != nil || !reflect.DeepEqual(before, after) {
		return m, errors.New("independent_post_window_identity_or_ledger_changed")
	}
	return m, nil
}
func verifyTarget(original, target material) error {
	if original.Format != target.Format || original.Kind != target.Kind || original.ToolSource != target.ToolSource || original.CSource != target.CSource || original.CResources != target.CResources || original.CResourceFiles != target.CResourceFiles || original.SessionZone != target.SessionZone || original.SessionCharset != target.SessionCharset || original.SessionSQLMode != target.SessionSQLMode || original.Binding != target.Binding || !bytes.Equal(original.MongoHead, target.MongoHead) || original.LedgerDDL != target.LedgerDDL || len(target.Ledger) != 1 || target.Ledger[0] != (ledgerRow{102, false}) {
		return errors.New("target_identity_or_ledger_rejected")
	}
	for i, a := range original.Tables {
		b := target.Tables[i]
		if a.Table != b.Table || !b.EOF || a.RetainedDDL != b.DDL || !reflect.DeepEqual(a.Columns, b.Columns) || len(a.Rows) != len(b.Rows) || a.Pages != b.Pages {
			return errors.New("target_retained_structure_or_eof_changed")
		}
		for n, row := range a.Rows {
			if row.Key != b.Rows[n].Key || row.Retained != b.Rows[n].Retained || b.Rows[n].Value != nil {
				return errors.New("target_retained_fact_changed")
			}
		}
	}
	return nil
}
