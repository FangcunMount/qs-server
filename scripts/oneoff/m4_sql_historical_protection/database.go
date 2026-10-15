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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const queryLimit = 5 * time.Second

var autoIncrementRE = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type database struct {
	conn               *sql.Conn
	mongo              *mongo.Client
	sqlName, mongoName string
	// nil in the CLI; native tests may stop after a real fixed ADD succeeds.
	afterRestoreDDL func(int)
}
type observation struct {
	SQLHash, MongoHash, MongoUUID string
	MongoHead                     []byte
	Version                       uint64
	Dirty                         bool
}

func normalizeDDL(ddl string, i int) string {
	var lines []string
	for _, line := range strings.Split(ddl, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "`"+objects[i].column+"` ") || strings.HasPrefix(line, "CONSTRAINT `"+objects[i].check+"` ") {
			continue
		}
		lines = append(lines, strings.TrimSuffix(autoIncrementRE.ReplaceAllString(line, ""), ","))
	}
	return strings.Join(lines, "\n")
}
func (d database) observe(ctx context.Context, r sqlReader) (observation, error) {
	var out observation
	if e := d.retiredObjectsMissing(ctx, r); e != nil {
		return out, e
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var uuid, name, zone, charset, mode, serverVersion string
	if r.QueryRowContext(q, "SELECT @@server_uuid,DATABASE(),@@session.time_zone,@@session.character_set_results,@@session.sql_mode,VERSION()").Scan(&uuid, &name, &zone, &charset, &mode, &serverVersion) != nil || !uuidRE.MatchString(uuid) || name != d.sqlName || zone != "+00:00" || charset != "utf8mb4" || mode != sessionMode || serverVersion != "8.0.36" {
		return out, errors.New("mysql_identity_or_session_rejected")
	}
	raw, _ := json.Marshal([]any{"mysql_server_selected_v1", uuid, name})
	out.SQLHash = fingerprint(raw)
	rows, e := r.QueryContext(q, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		return out, errors.New("sql_ledger_read_failed")
	}
	var count int
	for rows.Next() {
		if rows.Scan(&out.Version, &out.Dirty) != nil {
			_ = rows.Close()
			return out, errors.New("sql_ledger_invalid")
		}
		count++
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || count != 1 {
		return out, errors.New("sql_ledger_or_release_invalid")
	}
	var hello bson.Raw
	if d.mongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return out, errors.New("mongo_identity_unknown")
	}
	set, hosts, e := primaryReplicaIdentity(hello)
	if e != nil {
		return out, e
	}
	raw, _ = json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: set}, {Key: "hosts", Value: hosts}}, d.mongoName})
	out.MongoHash = fingerprint(raw)
	cur, e := d.mongo.Database(d.mongoName).Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		return out, errors.New("mongo_ledger_read_failed")
	}
	count = 0
	for cur.Next(q) {
		out.MongoHead = append([]byte(nil), cur.Current...)
		count++
	}
	re, ce = cur.Err(), cur.Close(q)
	if re != nil || ce != nil || count != 1 {
		return out, errors.New("mongo_ledger_or_release_invalid")
	}
	if !cleanMongo39(out.MongoHead) {
		return out, errors.New("mongo39_clean_required")
	}
	cur, e = d.mongo.Database(d.mongoName).ListCollections(q, bson.D{{Key: "name", Value: "schema_migrations"}})
	if e != nil {
		return out, errors.New("mongo_ledger_uuid_unknown")
	}
	count = 0
	for cur.Next(q) {
		v := cur.Current.Lookup("info", "uuid")
		subtype, b, e := v.BinaryOK()
		if !e || subtype != 4 || len(b) != 16 {
			_ = cur.Close(q)
			return out, errors.New("mongo_ledger_uuid_invalid")
		}
		out.MongoUUID = fingerprint(append([]byte{subtype}, b...))
		count++
	}
	re, ce = cur.Err(), cur.Close(q)
	if re != nil || ce != nil || count != 1 {
		return out, errors.New("mongo_uuid_or_release_invalid")
	}
	return out, nil
}

func primaryReplicaIdentity(raw bson.Raw) (string, []string, error) {
	primary, primaryOK := raw.Lookup("isWritablePrimary").BooleanOK()
	message := raw.Lookup("msg")
	if !primaryOK || !primary || (message.Type != 0 && (message.Type != bson.TypeString || message.StringValue() != "")) {
		return "", nil, errors.New("mongo_identity_unknown")
	}
	var hello struct {
		Set   string   `bson:"setName"`
		Hosts []string `bson:"hosts"`
	}
	if bson.Unmarshal(raw, &hello) != nil || hello.Set == "" || len(hello.Hosts) == 0 {
		return "", nil, errors.New("mongo_identity_unknown")
	}
	sort.Strings(hello.Hosts)
	for i, host := range hello.Hosts {
		if host == "" || (i > 0 && host == hello.Hosts[i-1]) {
			return "", nil, errors.New("mongo_identity_invalid")
		}
	}
	return hello.Set, hello.Hosts, nil
}

func (d database) retiredObjectsMissing(ctx context.Context, r sqlReader) error {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		return errors.New("legacy_scope_visibility_unknown")
	}
	visible := false
	for rows.Next() {
		var grant string
		if rows.Scan(&grant) != nil || strings.HasPrefix(grant, "REVOKE ") {
			_ = rows.Close()
			return errors.New("legacy_scope_visibility_unknown")
		}
		pieces := strings.SplitN(grant, " ON ", 2)
		if len(pieces) != 2 || !strings.HasPrefix(pieces[0], "GRANT ") {
			continue
		}
		scope := strings.SplitN(pieces[1], " TO ", 2)[0]
		if scope != "*.*" && scope != "`"+d.sqlName+"`.*" {
			continue
		}
		for _, priv := range strings.Split(strings.TrimPrefix(pieces[0], "GRANT "), ", ") {
			if priv == "SELECT" || priv == "ALL PRIVILEGES" {
				visible = true
			}
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !visible {
		return errors.New("legacy_scope_visibility_unknown")
	}
	var n int
	if r.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('domain_event_outbox','ai_bridge_commands','ai_messaging_legacy_commands')").Scan(&n) != nil || n != 0 {
		return errors.New("m3_retired_sql_object_reappeared_or_unknown")
	}
	cur, e := d.mongo.Database(d.mongoName).ListCollections(q, bson.D{{Key: "name", Value: "domain_event_outbox"}})
	if e != nil {
		return errors.New("m3_retired_mongo_scope_unknown")
	}
	found := cur.Next(q)
	re, ce = cur.Err(), cur.Close(q)
	if re != nil || ce != nil || found {
		return errors.New("m3_retired_mongo_object_reappeared_or_unknown")
	}
	return nil
}

func cleanMongo39(raw bson.Raw) bool {
	elements, e := raw.Elements()
	if e != nil {
		return false
	}
	seen := map[string]bool{}
	for _, element := range elements {
		if seen[element.Key()] {
			return false
		}
		seen[element.Key()] = true
	}
	v, ok := raw.Lookup("version").Int64OK()
	if !ok {
		n, valid := raw.Lookup("version").Int32OK()
		if !valid {
			return false
		}
		v = int64(n)
	}
	dirty, ok := raw.Lookup("dirty").BooleanOK()
	return v == 39 && ok && !dirty
}
func requireTransactionalTables(ctx context.Context, r sqlReader) error {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT TABLE_NAME,TABLE_TYPE,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('assessment','evaluation_outcome','schema_migrations') ORDER BY TABLE_NAME")
	if e != nil {
		return errors.New("transactional_table_metadata_failed")
	}
	names := []string{}
	for rows.Next() {
		var name, kind, engine string
		if rows.Scan(&name, &kind, &engine) != nil || kind != "BASE TABLE" || engine != "InnoDB" {
			_ = rows.Close()
			return errors.New("only_innodb_base_tables_allowed")
		}
		names = append(names, name)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !reflect.DeepEqual(names, []string{"assessment", "evaluation_outcome", "schema_migrations"}) {
		return errors.New("fixed_transactional_table_scope_unknown")
	}
	return nil
}

func ddl(ctx context.Context, r sqlReader, table string) (string, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var name, text string
	if r.QueryRowContext(q, "SHOW CREATE TABLE `"+table+"`").Scan(&name, &text) != nil || name != table || len(text) < 1 || len(text) > 65536 {
		return "", errors.New("fixed_table_ddl_read_failed")
	}
	return text, nil
}
func tableColumns(ctx context.Context, r sqlReader, i int) ([]column, bool, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", objects[i].table)
	if e != nil {
		return nil, false, errors.New("columns_read_failed")
	}
	var cols []column
	present := false
	for rows.Next() {
		var c column
		var nullable string
		if rows.Scan(&c.Name, &c.Type, &nullable) != nil || !nameRE.MatchString(c.Name) || len(c.Type) > 255 {
			_ = rows.Close()
			return nil, false, errors.New("column_invalid")
		}
		if c.Name == objects[i].column {
			if c.Type != "json" || nullable != "YES" {
				_ = rows.Close()
				return nil, false, errors.New("historical_type_rejected")
			}
			present = true
		} else {
			cols = append(cols, c)
		}
		if len(cols) > 128 {
			_ = rows.Close()
			return nil, false, errors.New("columns_limit")
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || len(cols) == 0 {
		return nil, false, errors.New("columns_release_failed")
	}
	rows, e = r.QueryContext(q, "SELECT CONSTRAINT_TYPE,ENFORCED FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME=? AND CONSTRAINT_NAME=?", objects[i].table, objects[i].check)
	if e != nil {
		return nil, false, errors.New("check_metadata_read_failed")
	}
	checks := 0
	for rows.Next() {
		var kind, enforced string
		if rows.Scan(&kind, &enforced) != nil || kind != "CHECK" || enforced != "YES" {
			_ = rows.Close()
			return nil, false, errors.New("check_metadata_rejected")
		}
		checks++
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil || ce != nil || checks != boolInt(present) {
		return nil, false, errors.New("partial_column_check_shape_rejected")
	}
	var pk string
	var nonUnique, parts int
	rows, e = r.QueryContext(q, "SELECT COLUMN_NAME,NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX", objects[i].table)
	if e != nil {
		return nil, false, errors.New("primary_key_read_failed")
	}
	for rows.Next() {
		if rows.Scan(&pk, &nonUnique) != nil {
			_ = rows.Close()
			return nil, false, errors.New("primary_key_invalid")
		}
		parts++
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil || ce != nil || parts != 1 || pk != "id" || nonUnique != 0 {
		return nil, false, errors.New("primary_key_or_release_rejected")
	}
	var idType string
	for _, c := range cols {
		if c.Name == "id" {
			idType = c.Type
		}
	}
	if idType != "bigint unsigned" {
		return nil, false, errors.New("primary_type_rejected")
	}
	return cols, present, nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func retainedHashSQL(cols []column) string {
	var encoded []string
	for _, c := range cols {
		name := "`" + c.Name + "`"
		prefix := c.Name + ":" + c.Type + ":"
		// The server converts temporal/decimal/JSON values under the bound session.
		// NULL and non-NULL are disjoint, lengths are byte lengths, bytes are HEX.
		encoded = append(encoded, fmt.Sprintf("X'%s',CASE WHEN %s IS NULL THEN 'N;' ELSE CONCAT('V',OCTET_LENGTH(CAST(%s AS BINARY)),':',HEX(CAST(%s AS BINARY)),';') END", hex.EncodeToString([]byte(prefix)), name, name, name))
	}
	return "SHA2(CONCAT(" + strings.Join(encoded, ",") + "),256)"
}
func scanTable(ctx context.Context, r sqlReader, i int, cols []column, present bool) (tableProof, error) {
	out := tableProof{Table: objects[i].table, Columns: cols, SQLNullOnly: present}
	var upper sql.NullString
	q, cancel := context.WithTimeout(ctx, queryLimit)
	e := r.QueryRowContext(q, "SELECT MAX(id) FROM `"+out.Table+"` FORCE INDEX(PRIMARY)").Scan(&upper)
	cancel()
	if e != nil {
		return out, errors.New("primary_range_read_failed")
	}
	max := uint64(0)
	if upper.Valid {
		max, e = strconv.ParseUint(upper.String, 10, 64)
		if e != nil {
			return out, errors.New("primary_range_invalid")
		}
	}
	var last uint64
	hasLast := false
	for {
		lower := "id>=0"
		args := []any{}
		if hasLast {
			lower = "id>CAST(? AS DECIMAL(20,0))"
			args = append(args, strconv.FormatUint(last, 10))
		}
		args = append(args, strconv.FormatUint(max, 10))
		null := "0"
		if present {
			null = "CASE WHEN `" + objects[i].column + "` IS NULL THEN 0 ELSE 1 END"
		}
		query := "SELECT /*+ MAX_EXECUTION_TIME(5000) */ id," + null + "," + retainedHashSQL(cols) + " FROM `" + out.Table + "` FORCE INDEX(PRIMARY) WHERE " + lower + " AND id<=CAST(? AS DECIMAL(20,0)) ORDER BY id LIMIT " + strconv.Itoa(pageSize)
		q, cancel = context.WithTimeout(ctx, queryLimit)
		rows, e := r.QueryContext(q, query, args...)
		if e != nil {
			cancel()
			return out, errors.New("bounded_page_read_failed")
		}
		n := 0
		for rows.Next() {
			var item rowProof
			var nonNull int
			if rows.Scan(&item.ID, &nonNull, &item.SHA256) != nil || nonNull != 0 || !hashRE.MatchString(item.SHA256) || (hasLast && item.ID <= last) || item.ID > max {
				_ = rows.Close()
				cancel()
				return out, errors.New("non_null_or_retained_row_rejected")
			}
			out.Rows = append(out.Rows, item)
			last = item.ID
			hasLast = true
			n++
			if len(out.Rows) > maxRows {
				_ = rows.Close()
				cancel()
				return out, errors.New("capture_row_limit_before_eof")
			}
		}
		re, ce := rows.Err(), rows.Close()
		cancel()
		if re != nil || ce != nil {
			return out, errors.New("page_release_failed")
		}
		out.Pages++
		if n == 0 {
			out.EOF = true
			break
		}
	}
	if e := verifyCompleteRangeCount(ctx, r, i, uint64(len(out.Rows))); e != nil {
		return out, e
	}
	return out, nil
}

// The independent whole-table count remains in the same read-only RR snapshot.
// Its aggregate work uses only the existing action's remaining absolute budget;
// pages, MAX(id), metadata, connection and session setup keep their 5s limits.
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
	if milliseconds > (20 * time.Minute).Milliseconds() {
		return errors.New("complete_range_count_budget_rejected")
	}
	var count uint64
	query := "SELECT /*+ MAX_EXECUTION_TIME(" + strconv.FormatInt(milliseconds, 10) + ") */ COUNT(*) FROM `" + objects[i].table + "`"
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
func (d database) snapshot(ctx context.Context, tool string, original bool) (m material, before observation, resultErr error) {
	if e := requireTransactionalTables(ctx, d.conn); e != nil {
		return m, observation{}, e
	}
	before, e := d.observe(ctx, d.conn)
	if e != nil {
		return m, before, e
	}
	if original && (before.Version != 100 || before.Dirty) {
		return m, before, errors.New("capture_requires_clean10039")
	}
	tx, e := d.conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return m, before, errors.New("repeatable_read_snapshot_failed")
	}
	done := false
	defer func() {
		if !done {
			recordRollback(&resultErr, tx)
		}
	}()
	m = material{Format: 1, Kind: "m4_a_sql_null_object_protection", ToolSource: tool, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), SQLName: d.sqlName, MongoName: d.mongoName, SQLIdentity: before.SQLHash, MongoIdentity: before.MongoHash, MongoUUID: before.MongoUUID, MongoHead: before.MongoHead, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode}
	m.LedgerDDL, e = ddl(ctx, tx, "schema_migrations")
	if e != nil {
		return m, before, e
	}
	ledgerRead, cancelLedgerRead := context.WithTimeout(ctx, queryLimit)
	defer cancelLedgerRead()
	ledgerRows, e := tx.QueryContext(ledgerRead, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		return m, before, errors.New("complete_ledger_snapshot_failed")
	}
	for ledgerRows.Next() {
		var item ledgerRow
		if ledgerRows.Scan(&item.Version, &item.Dirty) != nil {
			_ = ledgerRows.Close()
			return m, before, errors.New("complete_ledger_snapshot_invalid")
		}
		m.Ledger = append(m.Ledger, item)
	}
	re, ce := ledgerRows.Err(), ledgerRows.Close()
	cancelLedgerRead()
	if re != nil || ce != nil || len(m.Ledger) != 1 || m.Ledger[0] != (ledgerRow{before.Version, before.Dirty}) {
		return m, before, errors.New("complete_ledger_changed_or_release_failed")
	}
	ledgerSchemaRead, cancelLedgerSchemaRead := context.WithTimeout(ctx, queryLimit)
	defer cancelLedgerSchemaRead()
	ledgerColumns, e := tx.QueryContext(ledgerSchemaRead, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='schema_migrations' ORDER BY ORDINAL_POSITION")
	if e != nil {
		return m, before, errors.New("ledger_schema_read_failed")
	}
	var names []string
	for ledgerColumns.Next() {
		var name, kind, nullable string
		if ledgerColumns.Scan(&name, &kind, &nullable) != nil || nullable != "NO" || (name == "version" && kind != "bigint") || (name == "dirty" && kind != "tinyint(1)") {
			_ = ledgerColumns.Close()
			return m, before, errors.New("ledger_schema_invalid")
		}
		names = append(names, name)
	}
	re, ce = ledgerColumns.Err(), ledgerColumns.Close()
	cancelLedgerSchemaRead()
	if re != nil || ce != nil || !reflect.DeepEqual(names, []string{"version", "dirty"}) {
		return m, before, errors.New("unknown_ledger_schema_or_release_failed")
	}

	for i := range objects {
		cols, present, e := tableColumns(ctx, tx, i)
		if e != nil {
			return m, before, e
		}
		if original && !present {
			return m, before, errors.New("capture_requires_original_objects")
		}
		t, e := scanTable(ctx, tx, i, cols, present)
		if e != nil {
			return m, before, e
		}
		t.OriginalDDL, e = ddl(ctx, tx, objects[i].table)
		if e != nil {
			return m, before, e
		}
		if present {
			if _, e = validateOriginalObjectDefinition(t.OriginalDDL, i); e != nil {
				return m, before, e
			}
		}
		t.RetainedDDL = normalizeDDL(t.OriginalDDL, i)
		m.Tables[i] = t
	}
	if e = tx.Commit(); e != nil {
		return m, before, errors.New("read_snapshot_commit_unknown")
	}
	done = true
	after, e := d.observe(ctx, d.conn)
	if e != nil || !reflect.DeepEqual(before, after) {
		return m, before, errors.New("capture_identity_or_ledger_changed")
	}
	for i, t := range m.Tables {
		current, e := ddl(ctx, d.conn, t.Table)
		if e != nil || current != t.OriginalDDL {
			return m, before, errors.New("capture_ddl_changed")
		}
		_ = i
	}
	return m, before, nil
}
func compareRetained(want, got material) error {
	if want.SQLIdentity != got.SQLIdentity || want.MongoIdentity != got.MongoIdentity || want.MongoUUID != got.MongoUUID || !bytes.Equal(want.MongoHead, got.MongoHead) || want.LedgerDDL != got.LedgerDDL {
		return errors.New("material_identity_or_ledger_ddl_changed")
	}
	for i, w := range want.Tables {
		g := got.Tables[i]
		if w.RetainedDDL != g.RetainedDDL || !reflect.DeepEqual(w.Columns, g.Columns) || !reflect.DeepEqual(w.Rows, g.Rows) || !g.EOF {
			return errors.New("retained_structure_or_fact_changed")
		}
	}
	return nil
}
func (d database) verifyTarget(ctx context.Context, m material, tool string) error {
	got, o, e := d.snapshot(ctx, tool, false)
	if e != nil {
		return e
	}
	if o.Version != 101 || o.Dirty {
		return errors.New("target_clean10139_required")
	}
	if e = compareRetained(m, got); e != nil {
		return e
	}
	for _, t := range got.Tables {
		if t.SQLNullOnly {
			return errors.New("target_objects_still_present")
		}
	}
	return nil
}

// A lost COMMIT response remains unknown. This separate action proves the
// current original state from fresh reads, without replaying any recovery write.
func (d database) verifyRestored(ctx context.Context, m material, tool string) error {
	got, _, e := d.snapshot(ctx, tool, true)
	if e != nil {
		return e
	}
	return compareRestored(m, got)
}

func compareRestored(want, got material) error {
	if e := compareRetained(want, got); e != nil {
		return e
	}
	if !reflect.DeepEqual(want.Ledger, got.Ledger) {
		return errors.New("restored_original_ledger_rows_changed")
	}
	for i, t := range got.Tables {
		if !t.SQLNullOnly || t.OriginalDDL != want.Tables[i].OriginalDDL {
			return errors.New("restored_original_objects_or_ddl_changed")
		}
	}
	return nil
}

func restorationStatements(m material) ([2]string, error) {
	var out [2]string
	if fingerprint([]byte(originalDDL)) != originalDDLHash {
		return out, errors.New("compiled_original_ddl_changed")
	}
	a := strings.Index(originalDDL, "ALTER TABLE `assessment`")
	b := strings.Index(originalDDL, "ALTER TABLE `evaluation_outcome`")
	if a < 0 || b <= a {
		return out, errors.New("original_ddl_resource_invalid")
	}
	out[0] = originalDDL[a:b]
	out[1] = originalDDL[b:]
	for i := range out {
		encoding, e := validateOriginalObjectDefinition(m.Tables[i].OriginalDDL, i)
		if e != nil {
			return [2]string{}, e
		}
		var literal string
		switch encoding {
		case originalUTF8MB3:
			literal = "JSON_SCHEMA_VALID(_utf8mb3'"
		case originalUTF8MB4:
			literal = "JSON_SCHEMA_VALID(_utf8mb4'"
		default:
			return [2]string{}, errors.New("original_check_resource_invalid")
		}
		if strings.Count(out[i], "JSON_SCHEMA_VALID('") != 1 {
			return [2]string{}, errors.New("original_check_resource_invalid")
		}
		out[i] = strings.Replace(out[i], "JSON_SCHEMA_VALID('", literal, 1)
	}
	return out, nil
}
func (d database) restore(ctx context.Context, m material, a approval, tool, materialPath, materialHash, approvalPath, approvalHash string) (resultErr error) {
	if e := validateApproval(a, m, a.MaterialHash, tool, time.Now()); e != nil {
		return e
	}
	if m.CapturePath != materialPath {
		return errors.New("frozen_material_path_changed_before_claim")
	}
	if e := checkRecoveryBoundary(m, a, materialHash, false); e != nil {
		return e
	}
	got, o, e := d.snapshot(ctx, tool, false)
	if e != nil {
		return e
	}
	if o.Version != 101 {
		return errors.New("only101_pre_start_recovery_allowed")
	}
	if e = compareRetained(m, got); e != nil {
		return e
	}
	if got.Tables[0].SQLNullOnly && !got.Tables[1].SQLNullOnly {
		return errors.New("unproved_inverse_partial_restore_shape")
	}
	for i, t := range got.Tables {
		if t.SQLNullOnly {
			want, e := validateOriginalObjectDefinition(m.Tables[i].OriginalDDL, i)
			if e != nil {
				return e
			}
			current, e := validateOriginalObjectDefinition(t.OriginalDDL, i)
			if e != nil {
				return e
			}
			if current != want {
				return errors.New("existing_original_check_encoding_changed")
			}
		}
	}
	if _, e = readPrivate(materialPath, materialHash, maxMaterialBytes); e != nil {
		return e
	}
	raw, e := readPrivate(approvalPath, approvalHash, 32768)
	if e != nil {
		return e
	}
	var again approval
	if strictDecode(raw, &again) != nil || !reflect.DeepEqual(a, again) {
		return errors.New("restore_approval_changed_before_claim")
	}
	statements, e := restorationStatements(m)
	if e != nil {
		return e
	}
	if e = validateApproval(a, m, a.MaterialHash, tool, time.Now()); e != nil {
		return e
	}
	if e = consumeApproval(m, a, approvalHash); e != nil {
		return errors.New("restore_approval_consumption_failed")
	}
	// Undo the DROP prefix in reverse order so each acknowledged ADD leaves
	// another admitted prefix if the explicitly approved attempt is interrupted.
	for i := len(got.Tables) - 1; i >= 0; i-- {
		t := got.Tables[i]
		if e = validateMutableBoundary(m, a, materialHash, tool); e != nil {
			return e
		}
		if !t.SQLNullOnly {
			if _, e = d.conn.ExecContext(ctx, statements[i]); e != nil {
				return errors.New("object_ddl_restore_failed_or_unknown")
			}
			if d.afterRestoreDDL != nil {
				d.afterRestoreDDL(i)
			}
		}
	}
	restored, o, e := d.snapshot(ctx, tool, false)
	if e != nil {
		return e
	}
	if o.Version != 101 || o.Dirty != got.Ledger[0].Dirty {
		return errors.New("ledger_changed_before_replay")
	}
	if e = compareRetained(m, restored); e != nil {
		return e
	}
	for i, t := range restored.Tables {
		if !t.SQLNullOnly || t.OriginalDDL != m.Tables[i].OriginalDDL {
			return errors.New("original_objects_or_ddl_not_restored")
		}
	}
	if e = validateMutableBoundary(m, a, materialHash, tool); e != nil {
		return e
	}
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("ledger_transaction_begin_failed")
	}
	committed := false
	defer func() {
		if !committed {
			recordRollback(&resultErr, tx)
		}
	}()
	if e = validateMutableBoundary(m, a, materialHash, tool); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM schema_migrations"); e != nil {
		return errors.New("ledger_transaction_replay_failed")
	}
	for _, row := range m.Ledger {
		if e = validateMutableBoundary(m, a, materialHash, tool); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row.Version, row.Dirty); e != nil {
			return errors.New("ledger_transaction_replay_failed")
		}
	}
	if e = validateMutableBoundary(m, a, materialHash, tool); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return errors.New("ledger_commit_outcome_unknown")
	}
	committed = true
	final, _, e := d.snapshot(ctx, tool, true)
	if e != nil {
		return e
	}
	return compareRetained(m, final)
}

type transactionReleaseFailure struct{ operation error }

func (e *transactionReleaseFailure) Error() string { return e.operation.Error() }
func (e *transactionReleaseFailure) Unwrap() error { return e.operation }
func recordRollback(result *error, tx interface{ Rollback() error }) {
	if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
		if *result == nil {
			*result = errors.New("transaction_release_failed")
		}
		*result = &transactionReleaseFailure{operation: *result}
	}
}
