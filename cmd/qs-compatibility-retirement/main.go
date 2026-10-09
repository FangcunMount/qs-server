// qs-compatibility-retirement owns its connections. Inventory is read-only;
// fixed prepare captures and restores temporary assets without DROP authority.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
)

var sourceSHA string // Bound at build time, never supplied by a database helper.

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var runRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
var mysqlUUIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const maxRecords = 100000
const maxBytes = 128 << 20
const querySeconds = 15
const totalSeconds = 180

var targets = [][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}

type request struct {
	FormatVersion        int                                `json:"format_version"`
	Kind                 string                             `json:"kind"`
	OperationID          string                             `json:"operation_id"`
	SourceSHA            string                             `json:"source_sha"`
	TargetHash           string                             `json:"target_hash"`
	DatabaseScope        string                             `json:"database_scope"`
	Identities           map[string]string                  `json:"identity_hashes"`
	Migrations           map[string]uint64                  `json:"expected_migrations"`
	Limits               scanLimits                         `json:"limits"`
	BoundaryRunID        string                             `json:"boundary_run_id,omitempty"`
	BoundaryReportHash   string                             `json:"boundary_report_hash,omitempty"`
	Boundaries           []targetBoundary                   `json:"approved_boundaries,omitempty"`
	MongoNamespaceAnchor *identitymeta.MongoNamespaceAnchor `json:"mongodb_namespace_anchor,omitempty"`
}

type snapshot struct {
	Database          string            `json:"database"`
	Name              string            `json:"name"`
	Kind              string            `json:"kind"`
	Present           bool              `json:"present"`
	Complete          bool              `json:"complete"`
	Records           uint64            `json:"records"`
	SchemaHash        string            `json:"schema_hash"`
	DataHash          string            `json:"data_hash"`
	IdentityHash      string            `json:"identity_hash"`
	Bytes             uint64            `json:"bytes"`
	Classification    map[string]uint64 `json:"classification"`
	SourceFile        string            `json:"source_file,omitempty"`
	ErrorCategory     string            `json:"error_category"`
	Boundary          *targetBoundary   `json:"boundary,omitempty"`
	Passes            int               `json:"equal_full_passes"`
	Pages             uint64            `json:"pages"`
	NextCycleRequired bool              `json:"next_cycle_required"`
}
type databaseInventory struct {
	IdentityHash                 string                             `json:"identity_hash"`
	DatabaseAnchorHash           string                             `json:"database_anchor_hash"`
	NamespaceAnchor              *identitymeta.MongoNamespaceAnchor `json:"namespace_anchor,omitempty"`
	MigrationGenerationHash      string                             `json:"migration_generation_hash"`
	ExpectedIdentityMatch        bool                               `json:"expected_identity_match"`
	Version                      uint64                             `json:"migration_version"`
	Dirty                        bool                               `json:"migration_dirty"`
	ExpectedMigrationMatch       bool                               `json:"expected_migration_match"`
	CatalogHash                  string                             `json:"catalog_hash"`
	NonTargetSchemaHash          string                             `json:"non_target_schema_hash"`
	MetadataComplete             bool                               `json:"metadata_complete"`
	Permissions                  map[string]bool                    `json:"permissions"`
	OutsideDependencies          uint64                             `json:"outside_dependencies"`
	DependencyCoverageComplete   bool                               `json:"dependency_coverage_complete"`
	InboundFKCoverageComplete    bool                               `json:"inbound_foreign_key_coverage_complete"`
	DependencyScope              string                             `json:"dependency_scope"`
	DependencyTextReviewRequired bool                               `json:"dependency_text_review_required"`
	ErrorCategory                string                             `json:"error_category"`
}
type report struct {
	FormatVersion        int                          `json:"format_version"`
	Kind                 string                       `json:"kind"`
	SourceSHA            string                       `json:"source_sha"`
	OperationID          string                       `json:"operation_id"`
	RunID                string                       `json:"run_id"`
	RequestHash          string                       `json:"request_hash"`
	TargetHash           string                       `json:"target_hash"`
	ObservedAt           string                       `json:"observed_at"`
	Complete             bool                         `json:"complete"`
	DropReady            bool                         `json:"drop_ready"`
	DatabaseBindings     map[string]databaseInventory `json:"database_bindings"`
	Targets              []snapshot                   `json:"targets"`
	SourceBytesProtocol  string                       `json:"source_bytes_protocol"`
	ConsistencySemantics string                       `json:"consistency_semantics"`
	ErrorCategory        string                       `json:"error_category"`
	BoundaryReportHash   string                       `json:"boundary_report_hash,omitempty"`
	DiagnosticOnly       bool                         `json:"diagnostic_only"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func hashParts(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		frame(h, []byte(p), false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func frame(w io.Writer, value []byte, null bool) {
	var size [9]byte
	if !null {
		size[0] = 1
		binary.BigEndian.PutUint64(size[1:], uint64(len(value)))
	}
	_, _ = w.Write(size[:])
	if !null {
		_, _ = w.Write(value)
	}
}
func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, querySeconds*time.Second)
}
func category(value string) error { return errors.New(value) }
func envPort(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > 65535 {
		return 0, category("connection_input_invalid")
	}
	return n, nil
}
func privateDir(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return category("private_path_invalid")
	}
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return category("private_path_invalid")
		}
		if info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return category("private_ancestor_writable")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return category("private_directory_invalid")
	}
	return nil
}
func writeJSON(path string, value any) (err error) {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return category("private_output_exists_or_unavailable")
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = category("private_output_close_failed")
		}
	}()
	enc := json.NewEncoder(f)
	if enc.Encode(value) != nil || f.Sync() != nil {
		return category("private_output_failed")
	}
	directory, e := os.Open(filepath.Dir(path))
	if e != nil {
		return category("private_output_failed")
	}
	defer func() {
		if closeErr := directory.Close(); err == nil && closeErr != nil {
			err = category("private_output_close_failed")
		}
	}()
	if directory.Sync() != nil {
		return category("private_output_failed")
	}
	return nil
}
func readRequest(path, expected, op string) (request, error) {
	var r request
	if !hashRE.MatchString(expected) || !runRE.MatchString(op) || !shaRE.MatchString(sourceSHA) || (filepath.Base(path) != "inventory-request.json" && filepath.Base(path) != "boundary-request.json") {
		return r, category("request_binding_invalid")
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 1 || info.Size() > 256*1024 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return r, category("request_private_invalid")
	}
	b, e := os.ReadFile(path)
	if e != nil || digestRaw(b) != expected {
		return r, category("request_hash_mismatch")
	}
	if rejectDuplicateJSON(b) != nil || identitymeta.ValidateOptionalMongoNamespaceJSON(b, "mongodb_namespace_anchor") != nil {
		return r, category("request_schema_invalid")
	}
	// The Python supervisor performs duplicate-key and ownership validation too.
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
		return r, category("request_schema_invalid")
	}
	if r.OperationID != op || r.SourceSHA != sourceSHA || r.TargetHash != digest(targets) || r.DatabaseScope != "mysql-and-mongodb" || len(r.Identities) != 2 || len(r.Migrations) != 2 || !hashRE.MatchString(r.Identities["mysql"]) || !hashRE.MatchString(r.Identities["mongodb"]) || r.Identities["mysql"] == r.Identities["mongodb"] || r.Migrations["mysql"] == 0 || r.Migrations["mongodb"] == 0 {
		return r, category("request_binding_invalid")
	}
	if r.MongoNamespaceAnchor != nil && (r.FormatVersion != 2 || r.MongoNamespaceAnchor.Validate() != nil) {
		return r, category("mongo_namespace_anchor_rejected")
	}
	if r.FormatVersion == 2 {
		return r, validateV2Request(r, path)
	}
	if r.FormatVersion != 1 || r.Kind != "readonly_inventory_request" || filepath.Base(path) != "inventory-request.json" || r.Limits != legacyLimits() || r.BoundaryRunID != "" || r.BoundaryReportHash != "" || len(r.Boundaries) != 0 {
		return r, category("request_binding_invalid")
	}
	return r, nil
}

func rejectDuplicateJSON(raw []byte) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	var consume func() error
	consume = func() error {
		token, e := d.Token()
		if e != nil {
			return e
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return category("duplicate_json_key")
				}
				seen[name] = true
				if e = consume(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = consume(); e != nil {
					return e
				}
			}
		default:
			return category("invalid_json")
		}
		_, e = d.Token()
		return e
	}
	if e := consume(); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return category("invalid_json")
	}
	return nil
}
func digestRaw(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func mysqlOpen(ctx context.Context) (*sql.DB, error) {
	p, e := envPort("MYSQL_PORT", 3306)
	if e != nil {
		return nil, e
	}
	for _, k := range []string{"MYSQL_HOST", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE"} {
		if os.Getenv(k) == "" || strings.ContainsAny(os.Getenv(k), "\x00\r\n") {
			return nil, category("connection_input_invalid")
		}
	}
	c := mysql.NewConfig()
	c.User = os.Getenv("MYSQL_USERNAME")
	c.Passwd = os.Getenv("MYSQL_PASSWORD")
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(os.Getenv("MYSQL_HOST"), strconv.Itoa(p))
	c.DBName = os.Getenv("MYSQL_DATABASE")
	c.Timeout = 10 * time.Second
	c.ReadTimeout = 15 * time.Second
	c.WriteTimeout = 15 * time.Second
	c.MultiStatements = false
	c.ParseTime = false
	db, e := sql.Open("mysql", c.FormatDSN())
	if e != nil {
		return nil, category("mysql_connection_invalid")
	}
	db.SetMaxOpenConns(1)
	if db.PingContext(ctx) != nil {
		_ = db.Close() // Connection already failed; preserve the fixed primary error.
		return nil, category("mysql_connection_failed")
	}
	return db, nil
}
func scanSQL(ctx context.Context, tx *sql.Tx, query string, args ...any) ([][]*string, error) {
	q, cancel := queryContext(ctx)
	defer cancel()
	rows, e := tx.QueryContext(q, query, args...)
	if e != nil {
		return nil, category("mysql_metadata_read_failed")
	}
	defer func() { _ = rows.Close() }()
	cols, e := rows.Columns()
	if e != nil {
		return nil, category("mysql_metadata_read_failed")
	}
	out := make([][]*string, 0)
	for rows.Next() {
		if len(out) >= 10000 {
			return nil, category("metadata_bound_exceeded")
		}
		raw := make([]sql.RawBytes, len(cols))
		args := make([]any, len(cols))
		for i := range raw {
			args[i] = &raw[i]
		}
		if rows.Scan(args...) != nil {
			return nil, category("mysql_metadata_read_failed")
		}
		row := make([]*string, len(cols))
		for i, v := range raw {
			if v != nil {
				s := string(v)
				row[i] = &s
			}
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, category("mysql_metadata_read_failed")
	}
	return out, nil
}
func val(row []*string, i int) string {
	if i >= len(row) || row[i] == nil {
		return ""
	}
	return *row[i]
}
func quote(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

func mysqlCatalog(ctx context.Context, tx *sql.Tx) (map[string]string, map[string]any, error) {
	rows, e := scanSQL(ctx, tx, "SELECT TABLE_NAME,TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY TABLE_NAME")
	if e != nil {
		return nil, nil, e
	}
	kinds := map[string]string{}
	defs := map[string]any{}
	for _, r := range rows {
		n, k := val(r, 0), val(r, 1)
		kinds[n] = k
		create, e := scanSQL(ctx, tx, "SHOW CREATE TABLE "+quote(n))
		if e != nil || len(create) != 1 {
			return nil, nil, category("mysql_schema_read_failed")
		}
		// SHOW CREATE exposes a mutable next auto-increment value; it is not
		// schema and must not invalidate approved bounds when new rows arrive.
		for _, row := range create {
			if len(row) > 1 && row[1] != nil {
				normalized := regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`).ReplaceAllString(*row[1], "")
				row[1] = &normalized
			}
		}
		defs["table:"+n] = create
	}
	queries := []struct{ key, q string }{{"triggers", "SELECT TRIGGER_NAME,EVENT_OBJECT_TABLE,ACTION_STATEMENT FROM information_schema.triggers WHERE trigger_schema=DATABASE() ORDER BY TRIGGER_NAME"}, {"routines", "SELECT ROUTINE_NAME,ROUTINE_TYPE,ROUTINE_DEFINITION FROM information_schema.routines WHERE routine_schema=DATABASE() ORDER BY ROUTINE_NAME"}, {"events", "SELECT EVENT_NAME,EVENT_DEFINITION,STATUS FROM information_schema.events WHERE event_schema=DATABASE() ORDER BY EVENT_NAME"}, {"constraints", "SELECT TABLE_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_SCHEMA,REFERENCED_TABLE_NAME FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL ORDER BY TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION"}}
	queries = append(queries, struct{ key, q string }{"inbound_constraints", "SELECT TABLE_SCHEMA,TABLE_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_SCHEMA,REFERENCED_TABLE_NAME,COLUMN_NAME,REFERENCED_COLUMN_NAME FROM information_schema.key_column_usage WHERE REFERENCED_TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME IN ('domain_event_outbox','ai_bridge_commands','ai_messaging_legacy_commands') ORDER BY TABLE_SCHEMA,TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION"})
	for _, query := range queries {
		rows, e := scanSQL(ctx, tx, query.q)
		if e != nil {
			return nil, nil, e
		}
		defs[query.key] = rows
		for _, r := range rows {
			for _, v := range r {
				if v == nil && query.key != "constraints" {
					return nil, nil, category("mysql_definition_visibility_incomplete")
				}
			}
		}
	}
	if lenMustJSON(defs) > maxBytes {
		return nil, nil, category("metadata_bound_exceeded")
	}
	return kinds, defs, nil
}
func lenMustJSON(v any) int { b, _ := json.Marshal(v); return len(b) }
func targetSQL(name string) bool {
	return name == "domain_event_outbox" || name == "ai_bridge_commands" || name == "ai_messaging_legacy_commands"
}

func mysqlTarget(ctx context.Context, tx *sql.Tx, name, kind string, defs map[string]any, dir string, supplied ...request) (snapshot, error) {
	if len(supplied) == 1 && supplied[0].FormatVersion == 2 {
		return mysqlTargetV2(ctx, tx, name, kind, defs, dir, supplied[0])
	}
	s := snapshot{Database: "mysql", Name: name, Kind: "base_table", Present: kind != "", Complete: false, Classification: map[string]uint64{}, ErrorCategory: "none"}
	if kind == "" {
		s.Complete = true
		s.SchemaHash = digest(nil)
		s.DataHash = digest(nil)
		s.IdentityHash = hashParts("mysql-absent", name)
		return s, nil
	}
	if kind != "BASE TABLE" {
		return s, category("target_type_rejected")
	}
	s.SchemaHash = digest(defs["table:"+name])
	s.IdentityHash = hashParts("mysql-object-v1", name, s.SchemaHash)
	columns, e := scanSQL(ctx, tx, "SELECT COLUMN_NAME,COLUMN_TYPE FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", name)
	if e != nil || len(columns) == 0 {
		return s, category("target_columns_unavailable")
	}
	pks, e := scanSQL(ctx, tx, "SELECT COLUMN_NAME FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX", name)
	if e != nil || len(pks) != 1 || (name == "domain_event_outbox" && val(pks[0], 0) != "id") || (name != "domain_event_outbox" && val(pks[0], 0) != "command_id") {
		return s, category("target_primary_key_rejected")
	}
	queryCols := make([]string, len(columns))
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = val(c, 0)
		queryCols[i] = "CAST(" + quote(names[i]) + " AS BINARY)"
	}
	q, cancel := queryContext(ctx)
	defer cancel()
	rows, e := tx.QueryContext(q, "SELECT "+strings.Join(queryCols, ",")+" FROM "+quote(name)+" ORDER BY "+quote(val(pks[0], 0))+" LIMIT 100001")
	if e != nil {
		return s, category("target_read_failed")
	}
	defer func() { _ = rows.Close() }()
	filename := "mysql-" + name + ".source.ndjson"
	f, e := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return s, category("private_output_exists_or_unavailable")
	}
	defer func() { _ = f.Close() }() // Error-path cleanup; successful close is checked below.
	enc := json.NewEncoder(f)
	header := map[string]any{"protocol": "mysql_cast_binary_columns_pk_order_v1", "columns": columns}
	if enc.Encode(header) != nil {
		return s, category("private_output_failed")
	}
	h := sha256.New()
	frame(h, []byte(digest(columns)), false)
	for rows.Next() {
		if s.Records >= maxRecords {
			return s, category("target_record_bound_exceeded")
		}
		raw := make([]sql.RawBytes, len(columns))
		scan := make([]any, len(raw))
		for i := range raw {
			scan[i] = &raw[i]
		}
		if rows.Scan(scan...) != nil {
			return s, category("target_read_failed")
		}
		encoded := make([]*string, len(raw))
		for i, v := range raw {
			s.Bytes += uint64(len(v))
			if s.Bytes > maxBytes {
				return s, category("target_byte_bound_exceeded")
			}
			frame(h, v, v == nil)
			if v != nil {
				text := base64.StdEncoding.EncodeToString(v)
				encoded[i] = &text
			}
		}
		if enc.Encode(encoded) != nil {
			return s, category("private_output_failed")
		}
		s.Records++
		classifySQL(s.Classification, name, names, raw)
	}
	if rows.Err() != nil {
		return s, category("target_read_failed")
	}
	if f.Sync() != nil {
		return s, category("private_output_failed")
	}
	if f.Close() != nil {
		return s, category("private_output_close_failed")
	}
	s.DataHash = hex.EncodeToString(h.Sum(nil))
	s.SourceFile = filename
	s.Complete = true
	return s, nil
}
func classifySQL(count map[string]uint64, name string, columns []string, raw []sql.RawBytes) {
	if name == "ai_messaging_legacy_commands" {
		count["historical_mapping_requires_business_verification"]++
		return
	}
	key := "status"
	if name == "ai_bridge_commands" {
		key = "delivered"
	}
	value := ""
	for i, n := range columns {
		if n == key {
			value = string(raw[i])
		}
	}
	if name == "ai_bridge_commands" {
		switch value {
		case "1":
			count["delivered_requires_business_verification"]++
		case "0":
			count["undelivered"]++
		default:
			count["unknown_transport_state"]++
		}
		return
	}
	switch value {
	case "pending", "publishing", "failed", "published":
		count[value]++
	default:
		count["unknown_transport_state"]++
	}
}
func mysqlInventory(ctx context.Context, r request, dir string) (databaseInventory, []snapshot, error) {
	d := databaseInventory{Permissions: map[string]bool{}, DependencyTextReviewRequired: true, DependencyScope: "selected_schema_text_and_global_inbound_foreign_keys_dynamic_external_unproven", ErrorCategory: "none"}
	db, e := mysqlOpen(ctx)
	if e != nil {
		return d, nil, e
	}
	defer func() { _ = db.Close() }()
	tx, e := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return d, nil, category("mysql_readonly_transaction_failed")
	}
	defer func() { _ = tx.Rollback() }() // The transaction is explicitly read-only.
	ids, e := scanSQL(ctx, tx, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(ids) != 1 {
		return d, nil, category("mysql_identity_failed")
	}
	if val(ids[0], 1) != os.Getenv("MYSQL_DATABASE") || !strings.HasPrefix(val(ids[0], 2), "8.") {
		return d, nil, category("mysql_identity_or_version_rejected")
	}
	if !mysqlUUIDRE.MatchString(val(ids[0], 0)) {
		return d, nil, category("mysql_identity_or_version_rejected")
	}
	d.IdentityHash = hashParts("mysql_database_identity_v1", val(ids[0], 0), val(ids[0], 1))
	d.DatabaseAnchorHash = d.IdentityHash
	d.ExpectedIdentityMatch = d.IdentityHash == r.Identities["mysql"]
	if !d.ExpectedIdentityMatch {
		return d, nil, category("database_identity_mismatch")
	}
	grants, e := scanSQL(ctx, tx, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		return d, nil, e
	}
	for _, g := range grants {
		text := val(g, 0)
		if strings.HasPrefix(text, "REVOKE ") {
			d.Permissions["partial_revocations_present"] = true
		}
		if strings.Contains(text, " ON *.* TO ") && strings.HasPrefix(text, "GRANT ") {
			priv := strings.TrimPrefix(strings.Split(text, " ON *.* TO ")[0], "GRANT ")
			for _, needed := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT"} {
				if priv == "ALL PRIVILEGES" || containsPrivilege(priv, needed) {
					d.Permissions["global_"+strings.ToLower(strings.ReplaceAll(needed, " ", "_"))] = true
				}
			}
		}
	}
	d.Permissions["global_metadata_visibility"] = !d.Permissions["partial_revocations_present"] && d.Permissions["global_select"] && d.Permissions["global_show_view"] && d.Permissions["global_trigger"] && d.Permissions["global_event"]
	if !d.Permissions["global_metadata_visibility"] {
		return d, nil, category("mysql_global_metadata_visibility_unproven")
	}
	kinds, defs, e := mysqlCatalog(ctx, tx)
	if e != nil {
		return d, nil, e
	}
	heads, e := scanSQL(ctx, tx, "SELECT version,dirty FROM schema_migrations")
	if e != nil || len(heads) != 1 {
		return d, nil, category("mysql_migration_head_invalid")
	}
	d.Version, e = strconv.ParseUint(val(heads[0], 0), 10, 64)
	if e != nil || (val(heads[0], 1) != "0" && val(heads[0], 1) != "1") {
		return d, nil, category("mysql_migration_head_invalid")
	}
	d.Dirty = val(heads[0], 1) == "1"
	d.ExpectedMigrationMatch = d.Version == r.Migrations["mysql"]
	if d.Dirty || !d.ExpectedMigrationMatch {
		return d, nil, category("migration_head_rejected")
	}
	d.CatalogHash = digest(defs)
	non := map[string]any{}
	for k, v := range defs {
		if !strings.HasPrefix(k, "table:") || !targetSQL(strings.TrimPrefix(k, "table:")) {
			non[k] = v
		}
	}
	d.NonTargetSchemaHash = digest(non)
	for _, item := range defs["inbound_constraints"].([][]*string) {
		if val(item, 0) != os.Getenv("MYSQL_DATABASE") || !targetSQL(val(item, 1)) {
			d.OutsideDependencies++
		}
	}
	d.InboundFKCoverageComplete = true
	var snapshots []snapshot
	for _, t := range targets[:3] {
		s, e := mysqlTarget(ctx, tx, t[1], kinds[t[1]], defs, dir, r)
		if e != nil {
			s.ErrorCategory = e.Error()
			snapshots = append(snapshots, s)
			return d, snapshots, e
		}
		snapshots = append(snapshots, s)
	}
	_, end, e := mysqlCatalog(ctx, tx)
	if e != nil || digest(end) != d.CatalogHash {
		return d, snapshots, category("mysql_schema_changed_during_scan")
	}
	if e = writeJSON(filepath.Join(dir, "mysql-metadata.private.json"), map[string]any{"identity": ids, "grants_hash": digest(grants), "permission_facts": d.Permissions, "schema": defs}); e != nil {
		return d, snapshots, e
	}
	if r.FormatVersion == 2 && !boundaryMode(r) {
		// A concurrent post-upper insert is invisible in the old RR snapshot.
		// Close it before a separately bound live, read-only observation.
		if e = tx.Rollback(); e != nil {
			return d, snapshots, category("mysql_readonly_transaction_close_failed")
		}
		if e = mysqlObserveAfterUpper(ctx, db, r, d.CatalogHash, snapshots); e != nil {
			return d, snapshots, e
		}
	}
	d.MetadataComplete = true
	return d, snapshots, nil
}
func containsPrivilege(text, want string) bool {
	for _, p := range strings.Split(text, ",") {
		if strings.TrimSpace(p) == want {
			return true
		}
	}
	return false
}

func mongoOpen(ctx context.Context) (*mongo.Client, error) {
	p, e := envPort("MONGODB_PORT", 27017)
	if e != nil {
		return nil, e
	}
	for _, k := range []string{"MONGODB_HOST", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME"} {
		if os.Getenv(k) == "" || strings.ContainsAny(os.Getenv(k), "\x00\r\n") {
			return nil, category("connection_input_invalid")
		}
	}
	opts := options.Client().SetHosts([]string{net.JoinHostPort(os.Getenv("MONGODB_HOST"), strconv.Itoa(p))}).SetAuth(options.Credential{Username: os.Getenv("MONGODB_USERNAME"), Password: os.Getenv("MONGODB_PASSWORD"), AuthSource: "admin"}).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetSocketTimeout(15 * time.Second).SetMaxPoolSize(1).SetReadConcern(readconcern.Majority())
	c, e := mongo.Connect(ctx, opts)
	if e != nil {
		return nil, category("mongo_connection_failed")
	}
	if c.Ping(ctx, nil) != nil {
		_ = c.Disconnect(context.Background())
		return nil, category("mongo_connection_failed")
	}
	return c, nil
}
func canonicalBSON(raw bson.Raw) (any, error) {
	b, e := bson.MarshalExtJSON(raw, true, false)
	if e != nil {
		return nil, category("mongo_schema_decode_failed")
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return nil, category("mongo_schema_decode_failed")
	}
	return v, nil
}

// Diagnostic output contains only fixed tokens, a numeric server code and a
// namespace digest. Never format the driver error: it can contain credentials.
func mongoIndexDiagnosticLine(phase, namespace string, elapsed time.Duration, err error) string {
	switch phase {
	case "list", "iterate", "close":
	default:
		return ""
	}
	if err == nil {
		return ""
	}
	kind, code := mongoIndexErrorKind(err)
	if elapsed < 0 {
		elapsed = 0
	}
	namespaceHash := sha256.Sum256([]byte(namespace))
	return fmt.Sprintf("QS_MONGO_INDEX_DIAGNOSTIC phase=%s kind=%s code=%d namespace_sha256=%x elapsed_ms=%d", phase, kind, code, namespaceHash, elapsed.Milliseconds())
}

func mongoIndexErrorKind(err error) (string, int32) {
	var code int32
	server := false
	var pointer *mongo.CommandError
	if errors.As(err, &pointer) {
		// A typed-nil error cannot safely be unwrapped or treated as a server error.
		if pointer == nil {
			return "other", 0
		}
		code, server = pointer.Code, true
	} else {
		var value mongo.CommandError
		if errors.As(err, &value) {
			code, server = value.Code, true
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline", code
	}
	if errors.Is(err, context.Canceled) {
		return "context_cancelled", code
	}
	if server {
		return "server", code
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "network_timeout", 0
	}
	return "other", 0
}

func mongoSchemas(ctx context.Context, db *mongo.Database) (map[string]bson.Raw, map[string]any, error) {
	started := time.Now()
	q, cancel := queryContext(ctx)
	defer cancel()
	cur, e := db.ListCollections(q, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return nil, nil, category("mongo_catalog_read_failed")
	}
	defer func() { _ = cur.Close(ctx) }()
	raw := map[string]bson.Raw{}
	defs := map[string]any{}
	for cur.Next(q) {
		if len(raw) >= 500 {
			return nil, nil, category("metadata_bound_exceeded")
		}
		b := append(bson.Raw(nil), cur.Current...)
		n := b.Lookup("name").StringValue()
		raw[n] = b
		v, e := canonicalBSON(b)
		if e != nil {
			return nil, nil, e
		}
		defs["collection:"+n] = v
	}
	if cur.Err() != nil {
		return nil, nil, category("mongo_catalog_read_failed")
	}
	for n, b := range raw {
		if b.Lookup("type").StringValue() != "collection" {
			continue
		}
		indexes, e := db.Collection(n).Indexes().List(q)
		if e != nil {
			_, _ = fmt.Fprintln(os.Stderr, mongoIndexDiagnosticLine("list", db.Name()+"."+n, time.Since(started), e))
			return raw, defs, category("mongo_index_visibility_incomplete")
		}
		var defsIndex []any
		for indexes.Next(q) {
			v, e := canonicalBSON(indexes.Current)
			if e != nil {
				_ = indexes.Close(ctx)
				return raw, defs, e
			}
			defsIndex = append(defsIndex, v)
		}
		err := indexes.Err()
		phase := "iterate"
		closeErr := indexes.Close(ctx)
		if err == nil && closeErr != nil {
			err = closeErr
			phase = "close"
		}
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, mongoIndexDiagnosticLine(phase, db.Name()+"."+n, time.Since(started), err))
			return raw, defs, category("mongo_index_visibility_incomplete")
		}
		sort.Slice(defsIndex, func(i, j int) bool { return digest(defsIndex[i]) < digest(defsIndex[j]) })
		defs["indexes:"+n] = defsIndex
	}
	if lenMustJSON(defs) > maxBytes {
		return raw, defs, category("metadata_bound_exceeded")
	}
	return raw, defs, nil
}
func mongoScan(ctx context.Context, col *mongo.Collection, f *os.File) (uint64, uint64, string, map[string]uint64, error) {
	q, cancel := queryContext(ctx)
	defer cancel()
	cur, e := col.Find(q, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(maxRecords+1).SetBatchSize(100).SetMaxTime(querySeconds*time.Second))
	if e != nil {
		return 0, 0, "", nil, category("mongo_target_read_failed")
	}
	defer func() { _ = cur.Close(ctx) }()
	h := sha256.New()
	var count, size uint64
	classes := map[string]uint64{}
	for cur.Next(q) {
		if count >= maxRecords {
			return count, size, "", classes, category("target_record_bound_exceeded")
		}
		raw := cur.Current
		if e = raw.Validate(); e != nil {
			return count, size, "", classes, category("mongo_target_decode_failed")
		}
		size += uint64(len(raw))
		if size > maxBytes {
			return count, size, "", classes, category("target_byte_bound_exceeded")
		}
		frame(h, raw, false)
		if f != nil {
			var length [8]byte
			binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
			if _, e = f.Write(length[:]); e != nil {
				return count, size, "", classes, category("private_output_failed")
			}
			if _, e = f.Write(raw); e != nil {
				return count, size, "", classes, category("private_output_failed")
			}
		}
		count++
		value := raw.Lookup("status")
		if value.Type == bson.TypeString {
			switch state := value.StringValue(); state {
			case "pending", "publishing", "failed", "published", "quarantined":
				classes[state]++
			default:
				classes["unknown_transport_state"]++
			}
		} else {
			classes["unknown_transport_state"]++
		}
	}
	if cur.Err() != nil {
		return count, size, "", classes, category("mongo_target_read_failed")
	}
	return count, size, hex.EncodeToString(h.Sum(nil)), classes, nil
}
func mongoInventory(ctx context.Context, r request, dir string) (databaseInventory, snapshot, error) {
	d := databaseInventory{Permissions: map[string]bool{}, DependencyTextReviewRequired: true, ErrorCategory: "none"}
	s := snapshot{Database: "mongodb", Name: "domain_event_outbox", Kind: "collection", Classification: map[string]uint64{}, ErrorCategory: "none"}
	client, e := mongoOpen(ctx)
	if e != nil {
		return d, s, e
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(closeCtx)
	}()
	db := client.Database(os.Getenv("MONGODB_DBNAME"))
	var hello bson.Raw
	q, cancel := queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	cancel()
	if e != nil {
		return d, s, category("mongo_identity_read_failed")
	}
	collections, defs, e := mongoSchemas(ctx, db)
	if e != nil {
		return d, s, e
	}
	migration, ok := collections["schema_migrations"]
	if !ok || migration.Lookup("type").StringValue() != "collection" {
		return d, s, category("mongo_migration_head_invalid")
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		v := hello.Lookup(name)
		if v.Type != 0 {
			var a any
			if v.Unmarshal(&a) != nil {
				return d, s, category("mongo_identity_read_failed")
			}
			stable = append(stable, bson.E{Key: name, Value: a})
		}
	}
	// The schema_migrations collection UUID binds this selected database even
	// for a standalone server; no credentials or transient process ID is hashed.
	uuid := migration.Lookup("info", "uuid")
	if uuid.Type != bson.TypeBinary {
		return d, s, category("mongo_database_uuid_unavailable")
	}
	subtype, bytes := uuid.Binary()
	if subtype != 4 || len(bytes) != 16 {
		return d, s, category("mongo_database_uuid_unavailable")
	}
	stableJSON, _ := json.Marshal(stable)
	d.IdentityHash = hashParts("mongodb_database_identity_v1", string(stableJSON), os.Getenv("MONGODB_DBNAME"), hex.EncodeToString(bytes))
	if r.MongoNamespaceAnchor != nil {
		d.NamespaceAnchor, e = mongoNamespaceAnchor(ctx, db)
		if e != nil {
			return d, s, e
		}
		if !identitymeta.MatchMongoNamespaceAnchors(r.MongoNamespaceAnchor, d.NamespaceAnchor) {
			return d, s, category("mongo_namespace_anchor_mismatch")
		}
		d.DatabaseAnchorHash = d.NamespaceAnchor.Hash
	} else {
		d.DatabaseAnchorHash, e = mongoDatabaseAnchor(ctx, db, hello)
		if e != nil {
			return d, s, e
		}
	}
	d.MigrationGenerationHash, e = mongoMigrationGeneration(migration)
	if e != nil {
		return d, s, e
	}
	d.ExpectedIdentityMatch = d.IdentityHash == r.Identities["mongodb"]
	if !d.ExpectedIdentityMatch {
		return d, s, category("database_identity_mismatch")
	}
	var build struct {
		Version string `bson:"version"`
	}
	q, cancel = queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build)
	cancel()
	if e != nil || !strings.HasPrefix(build.Version, "7.") {
		return d, s, category("mongo_version_rejected")
	}
	q, cancel = queryContext(ctx)
	heads, e := db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		cancel()
		return d, s, category("mongo_migration_head_invalid")
	}
	var values []bson.M
	e = heads.All(q, &values)
	cancel()
	if e != nil || len(values) != 1 {
		return d, s, category("mongo_migration_head_invalid")
	}
	switch v := values[0]["version"].(type) {
	case int32:
		if v < 0 {
			return d, s, category("mongo_migration_head_invalid")
		}
		d.Version = uint64(v)
	case int64:
		if v < 0 {
			return d, s, category("mongo_migration_head_invalid")
		}
		d.Version = uint64(v)
	default:
		return d, s, category("mongo_migration_head_invalid")
	}
	dirty, ok := values[0]["dirty"].(bool)
	if !ok {
		return d, s, category("mongo_migration_head_invalid")
	}
	d.Dirty = dirty
	d.ExpectedMigrationMatch = d.Version == r.Migrations["mongodb"]
	if d.Dirty || !d.ExpectedMigrationMatch {
		return d, s, category("migration_head_rejected")
	}
	var privileges bson.Raw
	q, cancel = queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}}).Decode(&privileges)
	cancel()
	d.Permissions["effective_privileges_observed"] = e == nil
	if e != nil {
		return d, s, category("mongo_privileges_read_failed")
	}
	d.Permissions["list_collections_succeeded"] = true
	d.Permissions["all_indexes_succeeded"] = true
	d.CatalogHash = digest(defs)
	non := map[string]any{}
	for k, v := range defs {
		if k != "collection:domain_event_outbox" && k != "indexes:domain_event_outbox" {
			non[k] = v
		}
	}
	d.NonTargetSchemaHash = digest(non)
	if r.FormatVersion == 2 {
		s, e = mongoTargetV2(ctx, db, collections, defs, dir, r)
		if e != nil {
			return d, s, e
		}
	} else {
		b, present := collections[s.Name]
		s.Present = present
		if !present {
			s.SchemaHash = digest(nil)
			s.DataHash = digest(nil)
			s.IdentityHash = hashParts("mongodb-absent", s.Name)
			s.Complete = true
		} else {
			if b.Lookup("type").StringValue() != "collection" {
				return d, s, category("target_type_rejected")
			}
			s.SchemaHash = digest(map[string]any{"collection": defs["collection:"+s.Name], "indexes": defs["indexes:"+s.Name]})
			targetUUID := b.Lookup("info", "uuid")
			if targetUUID.Type != bson.TypeBinary {
				return d, s, category("mongo_target_uuid_unavailable")
			}
			_, v := targetUUID.Binary()
			s.IdentityHash = hashParts("mongodb-object-v1", hex.EncodeToString(v))
			s.SourceFile = "mongodb-domain_event_outbox.source.bsonframes"
			f, e := os.OpenFile(filepath.Join(dir, s.SourceFile), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return d, s, category("private_output_exists_or_unavailable")
			}
			s.Records, s.Bytes, s.DataHash, s.Classification, e = mongoScan(ctx, db.Collection(s.Name), f)
			syncErr := f.Sync()
			closeErr := f.Close()
			if e != nil {
				return d, s, e
			}
			if syncErr != nil || closeErr != nil {
				return d, s, category("private_output_failed")
			}
			n, size, hash, _, e := mongoScan(ctx, db.Collection(s.Name), nil)
			if e != nil || n != s.Records || size != s.Bytes || hash != s.DataHash {
				return d, s, category("mongo_source_changed_during_scan")
			}
			s.Complete = true
		}
	}
	if r.MongoNamespaceAnchor != nil {
		endAnchor, anchorErr := mongoNamespaceAnchor(ctx, db)
		if anchorErr != nil {
			return d, s, anchorErr
		}
		if !identitymeta.MatchMongoNamespaceAnchors(d.NamespaceAnchor, endAnchor) {
			return d, s, category("mongo_namespace_anchor_mismatch")
		}
	}
	_, end, e := mongoSchemas(ctx, db)
	if e != nil || digest(end) != d.CatalogHash {
		return d, s, category("mongo_schema_changed_during_scan")
	}
	if r.FormatVersion == 2 {
		var endHead struct {
			Version int64 `bson:"version"`
			Dirty   bool  `bson:"dirty"`
		}
		q, cancel = queryContext(ctx)
		var endRows []bson.Raw
		endCursor, headErr := db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
		if headErr == nil {
			headErr = endCursor.All(q, &endRows)
			closeErr := endCursor.Close(q)
			if headErr == nil {
				headErr = closeErr
			}
		}
		cancel()
		if headErr != nil || len(endRows) != 1 || bson.Unmarshal(endRows[0], &endHead) != nil || endRows[0].Lookup("dirty").Type != bson.TypeBoolean || endHead.Version < 0 || uint64(endHead.Version) != d.Version || endHead.Dirty {
			return d, s, category("mongo_migration_head_changed_during_scan")
		}
	}
	if e = writeJSON(filepath.Join(dir, "mongodb-metadata.private.json"), map[string]any{"hello": mustCanonical(hello), "effective_privileges_hash": digest(mustCanonical(privileges)), "permission_facts": d.Permissions, "schema": defs, "version": build.Version}); e != nil {
		return d, s, e
	}
	d.MetadataComplete = true
	return d, s, nil
}
func mustCanonical(raw bson.Raw) any { v, _ := canonicalBSON(raw); return v }

func run(requestPath, requestHash, op, runID, output string) (report, error) {
	result := report{FormatVersion: 1, Kind: "readonly_compatibility_inventory", SourceSHA: sourceSHA, OperationID: op, RunID: runID, RequestHash: requestHash, TargetHash: digest(targets), ObservedAt: time.Now().UTC().Format(time.RFC3339), DropReady: false, DiagnosticOnly: true, DatabaseBindings: map[string]databaseInventory{}, SourceBytesProtocol: "mysql_cast_binary_columns_pk_order_v1+mongodb_server_bson_pk_order_v1", ConsistencySemantics: "mysql_readonly_repeatable_read_and_schema_reobserve;mongo_two_equal_full_scans_and_schema_reobserve", ErrorCategory: "inventory_incomplete"}
	if !runRE.MatchString(runID) || privateDir(filepath.Dir(requestPath)) != nil || privateDir(output) != nil {
		return result, category("input_or_private_path_invalid")
	}
	r, e := readRequest(requestPath, requestHash, op)
	if e != nil {
		return result, e
	}
	if r.FormatVersion == 2 {
		result.FormatVersion = 2
		result.BoundaryReportHash = r.BoundaryReportHash
		result.SourceBytesProtocol = "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2"
		result.ConsistencySemantics = "two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced"
	}
	filename := "inventory.private.json"
	if boundaryMode(r) {
		result.Kind = "readonly_inventory_boundaries"
		filename = "boundary.private.json"
		result.SourceBytesProtocol = "no_source_body_copy"
		result.ConsistencySemantics = "diagnostic_upper_discovery_requires_independent_request_approval"
	}
	runStarted := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.Limits.TotalSeconds)*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, scanRunKey{}, scanRunIdentity{OperationID: op, RunID: runID, RequestHash: requestHash})
	d, sqlTargets, sqlErr := mysqlInventory(ctx, r, output)
	if sqlErr != nil {
		d.ErrorCategory = sqlErr.Error()
	}
	result.DatabaseBindings["mysql"] = d
	result.Targets = append(result.Targets, sqlTargets...)
	ctx = context.WithValue(ctx, scanTimingKey{}, scanTiming{RunStarted: runStarted, MySQLFinished: time.Now()})
	m, mongoTarget, mongoErr := mongoInventory(ctx, r, output)
	if mongoErr != nil {
		m.ErrorCategory = mongoErr.Error()
		mongoTarget.ErrorCategory = mongoErr.Error()
	}
	result.DatabaseBindings["mongodb"] = m
	result.Targets = append(result.Targets, mongoTarget)
	if sqlErr == nil && mongoErr == nil && len(result.Targets) == 4 {
		result.Complete = true
		result.ErrorCategory = "none"
	}
	if e = writeJSON(filepath.Join(output, filename), result); e != nil {
		return result, e
	}
	if !result.Complete {
		return result, category("inventory_incomplete")
	}
	return result, nil
}

// The safe stdout summary is counts/hashes only. Payloads and metadata/account
// names stay in the operation's private directory, never in an Action artifact.
func safeSummary(r report) map[string]any {
	objects := make([]map[string]any, 0, len(r.Targets))
	for _, s := range r.Targets {
		objects = append(objects, map[string]any{"database": s.Database, "name": s.Name, "present": s.Present, "complete": s.Complete, "records": s.Records, "bytes": s.Bytes, "schema_hash": s.SchemaHash, "data_hash": s.DataHash, "identity_hash": s.IdentityHash, "classification": s.Classification, "error_category": s.ErrorCategory, "boundary_hash": digest(s.Boundary), "equal_full_passes": s.Passes, "pages": s.Pages, "next_cycle_required": s.NextCycleRequired})
	}
	encoded, _ := json.Marshal(r)
	return map[string]any{"format_version": r.FormatVersion, "kind": r.Kind, "source_sha": r.SourceSHA, "operation_id": r.OperationID, "run_id": r.RunID, "request_hash": r.RequestHash, "target_hash": r.TargetHash, "complete": r.Complete, "drop_ready": false, "diagnostic_only": true, "boundary_report_hash": r.BoundaryReportHash, "error_category": r.ErrorCategory, "database_bindings": r.DatabaseBindings, "targets": objects, "private_report_hash": digestRaw(append(encoded, '\n'))}
}
func main() {
	if len(os.Args) == 3 && os.Args[1] == "--restore-wire" {
		if runLifecycleRestoreWire(os.Args[2]) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--source-sha" {
		fmt.Println(sourceSHA)
		return
	}
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	req := fs.String("request", "", "private request")
	hash := fs.String("request-hash", "", "exact request hash")
	op := fs.String("operation-id", "", "operation binding")
	runID := fs.String("run-id", "", "producer binding")
	out := fs.String("output-directory", "", "new private run directory")
	mode := fs.String("mode", "inventory", "fixed inventory or identity metadata mode")
	if fs.Parse(os.Args[1:]) != nil || fs.NArg() != 0 {
		fmt.Println(`{"format_version":1,"complete":false,"drop_ready":false,"error_category":"input_invalid"}`)
		os.Exit(1)
	}
	if *mode == "host-budget-key-create" || *mode == "host-budget-key-open" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		r, err := runLifecycleServiceKey(ctx, *mode, *req, *hash, *op, *runID)
		cancel()
		if json.NewEncoder(os.Stdout).Encode(r) != nil || err != nil {
			os.Exit(1)
		}
		return
	}
	if *mode == "host-services-d" || *mode == "host-services-d-recovery" || *mode == "host-services-d-template" || *mode == "host-services-d-recovery-template" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := runLifecycleServiceSession(ctx, *mode, *req, *hash, *op, *runID, os.Stdin, os.Stdout)
		cancel()
		if err != nil {
			os.Exit(1)
		}
		return
	}
	if *mode == "lifecycle-prepare-root-once" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		staged, e := stageLifecycleRootInputs(ctx, *req, *hash, *op, *runID)
		r := lifecycleReceipt{FormatVersion: 1, Kind: "compatibility_retirement_lifecycle_result", Operation: "prepare", SourceSHA: sourceSHA, OperationID: *op, RunID: *runID, RequestSHA256: *hash, TargetHash: digest(targets), TargetCount: 4}
		if e == nil {
			r, e = runLifecycleCLI(ctx, "lifecycle-prepare", staged, *hash, *op, *runID)
		} else {
			r.ErrorCategory = lifecycleCategory(e)
		}
		cancel()
		if json.NewEncoder(os.Stdout).Encode(r) != nil || e != nil {
			os.Exit(1)
		}
		return
	}
	if strings.HasPrefix(*mode, "lifecycle-") {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		r, e := runLifecycleCLI(ctx, *mode, *req, *hash, *op, *runID)
		cancel()
		if json.NewEncoder(os.Stdout).Encode(r) != nil || e != nil {
			os.Exit(1)
		}
		return
	}
	if *mode == "identity" {
		r, e := runIdentity(*req, *hash, *op, *runID, *out)
		if e != nil {
			r.ErrorCategory = e.Error()
			r.Complete = false
		}
		_ = json.NewEncoder(os.Stdout).Encode(identitySummary(r))
		if e != nil {
			os.Exit(1)
		}
		return
	}
	if *mode != "inventory" && *mode != "bounds" {
		fmt.Println(`{"format_version":1,"complete":false,"drop_ready":false,"error_category":"input_invalid"}`)
		os.Exit(1)
	}
	if r, e := readRequest(*req, *hash, *op); e == nil && productionRequestClass(r, *mode) != nil {
		fmt.Println(`{"format_version":1,"complete":false,"drop_ready":false,"error_category":"request_class_or_version_rejected"}`)
		os.Exit(1)
	}
	r, e := run(*req, *hash, *op, *runID, *out)
	if e != nil {
		r.ErrorCategory = e.Error()
		r.Complete = false
	}
	_ = json.NewEncoder(os.Stdout).Encode(safeSummary(r))
	if e != nil {
		os.Exit(1)
	}
}
