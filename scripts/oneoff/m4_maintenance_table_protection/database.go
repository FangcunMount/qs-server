package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
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

func primaryReplicaHosts(raw bson.Raw) ([]string, error) {
	set, setOK := raw.Lookup("setName").StringValueOK()
	primary, primaryOK := raw.Lookup("isWritablePrimary").BooleanOK()
	message := raw.Lookup("msg")
	if set != "rs0" || !setOK || !primaryOK || !primary || (message.Type != 0 && (message.Type != bson.TypeString || message.StringValue() != "")) {
		return nil, errors.New("mongo_cluster_unknown")
	}
	a, ok := raw.Lookup("hosts").ArrayOK()
	if !ok {
		return nil, errors.New("mongo_cluster_unknown")
	}
	values, e := a.Values()
	if e != nil || len(values) == 0 || len(values) > 32 {
		return nil, errors.New("mongo_cluster_unknown")
	}
	hosts := make([]string, 0, len(values))
	for _, value := range values {
		host, ok := value.StringValueOK()
		if !ok || host == "" {
			return nil, errors.New("mongo_cluster_invalid")
		}
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for i := 1; i < len(hosts); i++ {
		if hosts[i] == hosts[i-1] {
			return nil, errors.New("mongo_cluster_invalid")
		}
	}
	return hosts, nil
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
	hello, e := d.mongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Raw()
	cancel()
	if e != nil {
		return out, errors.New("mongo_cluster_unknown")
	}
	hosts, e := primaryReplicaHosts(hello)
	if e != nil {
		return out, e
	}
	raw, _ = json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: "rs0"}, {Key: "hosts", Value: hosts}}, d.binding.MongoName})
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

func requireTables(ctx context.Context, r sqlReader, original bool) error {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT TABLE_NAME,TABLE_TYPE,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('testee_store_migration_reversions','testee_store_migration_items','testee_store_migration_manifests','clinician_operator_binding_archive','operator_retirement_manifests','schema_migrations') ORDER BY TABLE_NAME")
	if e != nil {
		return errors.New("fixed_table_metadata_unavailable")
	}
	var got []string
	for rows.Next() {
		var name, kind, engine string
		if rows.Scan(&name, &kind, &engine) != nil || kind != "BASE TABLE" || engine != "InnoDB" {
			_ = rows.Close()
			return errors.New("fixed_innodb_base_tables_required")
		}
		got = append(got, name)
	}
	re, ce := rows.Err(), rows.Close()
	want := []string{"schema_migrations"}
	if original {
		for _, s := range scope {
			want = append(want, s.name)
		}
		sort.Strings(want)
	}
	if re != nil || ce != nil || !sameStrings(got, want) {
		return errors.New("fixed_table_presence_or_release_rejected")
	}
	return nil
}
func ddl(ctx context.Context, r sqlReader, table string) (string, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var name, text string
	if r.QueryRowContext(q, "SHOW CREATE TABLE `"+table+"`").Scan(&name, &text) != nil || name != table || text == "" || len(text) > maxDDLBytes {
		return "", errors.New("fixed_table_ddl_unavailable")
	}
	return text, nil
}
func columns(ctx context.Context, r sqlReader, i int) ([]column, error) {
	s := scope[i]
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,GENERATION_EXPRESSION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", s.name)
	if e != nil {
		return nil, errors.New("fixed_column_metadata_unavailable")
	}
	var got []column
	for rows.Next() {
		var c column
		var nullable, extra, generated string
		var def sql.NullString
		if rows.Scan(&c.Name, &c.Type, &nullable, &def, &extra, &generated) != nil || (nullable != "YES" && nullable != "NO") || extra != "" || generated != "" || def.Valid != (i == 2 && (c.Name == "after_hash" || c.Name == "rollback_hash")) || def.Valid && def.String != "" {
			_ = rows.Close()
			return nil, errors.New("fixed_column_shape_rejected")
		}
		c.Nullable = nullable == "YES"
		got = append(got, c)
		if len(got) > len(s.columns) {
			_ = rows.Close()
			return nil, errors.New("fixed_column_scope_rejected")
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !sameColumns(got, s.columns) {
		return nil, errors.New("fixed_columns_or_release_rejected")
	}
	rows, e = r.QueryContext(q, "SELECT COLUMN_NAME,NON_UNIQUE,SUB_PART,COLLATION,EXPRESSION,INDEX_TYPE,IS_VISIBLE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX", s.name)
	if e != nil {
		return nil, errors.New("primary_key_metadata_unavailable")
	}
	var keys []string
	valid := true
	for rows.Next() {
		var key string
		var nonUnique int
		var prefix sql.NullInt64
		var expression sql.NullString
		var order, kind, visible string
		if rows.Scan(&key, &nonUnique, &prefix, &order, &expression, &kind, &visible) != nil || nonUnique != 0 || prefix.Valid || expression.Valid || order != "A" || kind != "BTREE" || visible != "YES" {
			valid = false
		}
		keys = append(keys, key)
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil || ce != nil || !valid || !sameStrings(keys, s.keys) {
		return nil, errors.New("fixed_primary_key_or_release_rejected")
	}
	return got, nil
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
func pageSQL(i int, hasLast bool) string {
	s := scope[i]
	var selectParts, order []string
	where := "1=1"
	for n, key := range s.keys {
		expr := "`" + key + "`"
		if n == 0 && s.binaryFirst {
			selectParts = append(selectParts, "HEX(CAST("+expr+" AS BINARY))")
			order = append(order, "CAST("+expr+" AS BINARY)")
		} else {
			selectParts = append(selectParts, "CAST("+expr+" AS CHAR)")
			order = append(order, expr)
		}
	}
	if hasLast {
		switch {
		case len(s.keys) == 2:
			where = "(CAST(`migration_id` AS BINARY)>UNHEX(?) OR (CAST(`migration_id` AS BINARY)=UNHEX(?) AND `testee_id`>CAST(? AS DECIMAL(20,0))))"
		case s.binaryFirst:
			where = "CAST(`migration_id` AS BINARY)>UNHEX(?)"
		default:
			where = "`clinician_id`>CAST(? AS DECIMAL(20,0))"
		}
	}
	var lengths, values []string
	for _, c := range s.columns {
		col := "`" + c.Name + "`"
		lengths = append(lengths, "OCTET_LENGTH(CAST("+col+" AS BINARY))")
	}
	rowSize := "(" + strings.Join(func() []string {
		a := make([]string, len(lengths))
		for n, v := range lengths {
			a[n] = "COALESCE(" + v + ",0)"
		}
		return a
	}(), "+") + ")"
	// Test lengths before materializing either the original bytes or the
	// HEX/CONCAT hash frame. An oversized LONGTEXT never enters that branch.
	var bounded []string
	for _, length := range lengths {
		bounded = append(bounded, "COALESCE("+length+",0)<="+strconv.Itoa(maxValueBytes))
	}
	bounded = append(bounded, rowSize+"<="+strconv.Itoa(maxRowBytes))
	allowed := strings.Join(bounded, " AND ")
	for _, c := range s.columns {
		values = append(values, "CASE WHEN "+allowed+" THEN CAST(`"+c.Name+"` AS BINARY) ELSE NULL END")
	}
	selectParts = append(selectParts, lengths...)
	selectParts = append(selectParts, values...)
	selectParts = append(selectParts, "CASE WHEN "+allowed+" THEN "+retainedHashSQL(s.columns)+" ELSE NULL END")
	return "SELECT /*+ MAX_EXECUTION_TIME(5000) */ " + strings.Join(selectParts, ",") + " FROM `" + s.name + "` FORCE INDEX(PRIMARY) WHERE " + where + " ORDER BY " + strings.Join(order, ",") + " LIMIT " + strconv.Itoa(pageSize)
}
func pageArgs(s tableSpec, key []string) []any {
	if len(key) == 0 {
		return nil
	}
	if len(s.keys) == 2 {
		return []any{key[0], key[0], key[1]}
	}
	return []any{key[0]}
}
func scanPageRow(rows *sql.Rows, s tableSpec) (rowProof, error) {
	out := rowProof{Key: make([]string, len(s.keys)), Values: make([]cell, len(s.columns))}
	lengths := make([]sql.NullInt64, len(s.columns))
	raw := make([][]byte, len(s.columns))
	args := make([]any, 0, len(s.keys)+2*len(s.columns)+1)
	for n := range out.Key {
		args = append(args, &out.Key[n])
	}
	for n := range lengths {
		args = append(args, &lengths[n])
	}
	for n := range raw {
		args = append(args, &raw[n])
	}
	var hash sql.NullString
	args = append(args, &hash)
	if rows.Scan(args...) != nil {
		return out, errors.New("bounded_row_scan_failed")
	}
	size := 0
	for n, v := range raw {
		l := lengths[n]
		if l.Valid && (l.Int64 < 0 || l.Int64 > maxValueBytes || v == nil || l.Int64 != int64(len(v))) || !l.Valid && v != nil {
			return out, errors.New("cell_value_limit_or_null_rejected")
		}
		out.Values[n] = cell{SQLNull: !l.Valid}
		if l.Valid {
			out.Values[n].Bytes = append([]byte{}, v...)
		}
		size += len(v)
	}
	if size > maxRowBytes {
		return out, errors.New("row_value_limit_rejected")
	}
	if !hash.Valid {
		return out, errors.New("bounded_row_hash_missing")
	}
	out.Hash = hash.String
	if e := validateRow(s, out); e != nil {
		return out, e
	}
	return out, nil
}
func scanTable(ctx context.Context, r sqlReader, i int, cols []column, totalRows, totalBytes *int) (tableProof, error) {
	s := scope[i]
	out := tableProof{Table: s.name, Columns: cols, Keys: append([]string(nil), s.keys...)}
	var last []string
	for {
		q, cancel := context.WithTimeout(ctx, queryLimit)
		rows, e := r.QueryContext(q, pageSQL(i, last != nil), pageArgs(s, last)...)
		if e != nil {
			cancel()
			return out, errors.New("bounded_primary_page_unavailable")
		}
		count := 0
		for rows.Next() {
			row, e := scanPageRow(rows, s)
			if e != nil {
				_ = rows.Close()
				cancel()
				return out, e
			}
			if last != nil && !keyLess(s, last, row.Key) {
				_ = rows.Close()
				cancel()
				return out, errors.New("primary_page_order_rejected")
			}
			raw, e := json.Marshal(row)
			if e != nil {
				_ = rows.Close()
				cancel()
				return out, errors.New("row_encoding_failed")
			}
			if e := accountRow(len(raw)+1, len(out.Rows), &out.EncodedRowsBytes, totalRows, totalBytes); e != nil {
				_ = rows.Close()
				cancel()
				return out, e
			}
			out.Rows = append(out.Rows, row)
			last = append([]string(nil), row.Key...)
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
	if e := verifyCompleteRangeCount(ctx, r, i, uint64(len(out.Rows))); e != nil {
		return out, e
	}
	return out, nil
}

// Account the exact row JSON frame before adding it to the material. Rejection
// never marks EOF or silently truncates an over-limit table.
func accountRow(encoded, tableRows int, tableBytes, totalRows, totalBytes *int) error {
	if encoded <= 0 || encoded > maxTableBytes-*tableBytes || encoded > maxMaterialBytes-*totalBytes || tableRows >= maxRows || *totalRows >= maxTotalRows {
		return errors.New("material_limit_before_mandatory_eof")
	}
	*tableBytes += encoded
	*totalBytes += encoded
	*totalRows++
	return nil
}

// This independent whole-table count remains on the caller's read-only RR
// snapshot. Only this terminal aggregate uses the remaining action deadline;
// ordinary metadata, keyset pages and connection/session setup keep 5s limits.
func verifyCompleteRangeCount(ctx context.Context, r sqlReader, i int, expected uint64) error {
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return errors.New("complete_range_count_budget_rejected")
	}
	if e := ctx.Err(); e != nil {
		return classifyCompleteCountError(e)
	}
	milliseconds := time.Until(deadline).Milliseconds()
	if milliseconds <= 0 {
		return errors.New("complete_range_count_deadline_exceeded")
	}
	if milliseconds > (maxAction).Milliseconds() {
		return errors.New("complete_range_count_budget_rejected")
	}
	var count uint64
	query := "SELECT /*+ MAX_EXECUTION_TIME(" + strconv.FormatInt(milliseconds, 10) + ") */ COUNT(*) FROM `" + scope[i].name + "`"
	if e := r.QueryRowContext(ctx, query).Scan(&count); e != nil {
		return classifyCompleteCountError(e)
	}
	if count != expected {
		return errors.New("complete_range_count_mismatch")
	}
	return nil
}

func classifyCompleteCountError(e error) error {
	var server *mysql.MySQLError
	switch {
	case errors.As(e, &server) && server.Number == 3024:
		return errors.New("complete_range_count_server_timeout")
	case errors.Is(e, context.DeadlineExceeded):
		return errors.New("complete_range_count_deadline_exceeded")
	case errors.Is(e, context.Canceled):
		return errors.New("complete_range_count_canceled")
	default:
		return errors.New("complete_range_count_query_failed")
	}
}

func (d database) snapshot(ctx context.Context, tool string, version uint64, full bool) (m material, resultErr error) {
	original := version == 102
	if e := requireTables(ctx, d.conn, original); e != nil {
		return m, e
	}
	before, e := d.observe(ctx, d.conn)
	if e != nil {
		return m, e
	}
	if before.Ledger != (ledgerRow{version, false}) {
		return m, errors.New("clean102_or103_expected_for_action")
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
	m = material{Format: 1, Kind: "m4_d_maintenance_table_protection", ToolSource: tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Binding: before.Binding, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, MongoHead: before.Mongo}
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
	if e := requireTables(ctx, tx, original); e != nil {
		return m, e
	}
	totalRows, totalBytes := 0, 0
	if original {
		for i, s := range scope {
			cols, e := columns(ctx, tx, i)
			if e != nil {
				return m, e
			}
			t := tableProof{Table: s.name, Columns: cols, Keys: append([]string(nil), s.keys...)}
			if full {
				t, e = scanTable(ctx, tx, i, cols, &totalRows, &totalBytes)
				if e != nil {
					return m, e
				}
			}
			t.DDL, e = ddl(ctx, tx, s.name)
			if e != nil {
				return m, e
			}
			m.Tables[i] = t
		}
	}
	if tx.Commit() != nil {
		return m, errors.New("readonly_transaction_commit_unproven")
	}
	done = true
	after, e := d.observe(ctx, d.conn)
	if e != nil || !reflect.DeepEqual(before, after) {
		return m, errors.New("independent_post_window_identity_or_ledger_changed")
	}
	if e := requireTables(ctx, d.conn, original); e != nil {
		return m, e
	}
	return m, nil
}
func verifyTarget(original, target material) error {
	if original.Format != target.Format || original.Kind != target.Kind || original.ToolSource != target.ToolSource || original.DSource != target.DSource || original.DResources != target.DResources || original.DResourceFiles != target.DResourceFiles || original.SessionZone != target.SessionZone || original.SessionCharset != target.SessionCharset || original.SessionSQLMode != target.SessionSQLMode || original.Binding != target.Binding || !bytes.Equal(original.MongoHead, target.MongoHead) || original.LedgerDDL != target.LedgerDDL || len(target.Ledger) != 1 || target.Ledger[0] != (ledgerRow{103, false}) {
		return errors.New("target_identity_or_ledger_rejected")
	}
	for _, t := range target.Tables {
		if t.Table != "" || len(t.Rows) != 0 {
			return errors.New("target_legacy_table_reappeared")
		}
	}
	return nil
}
