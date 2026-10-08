package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

const maxDumpBytes int64 = 20 << 30
const maxJSONBytes int64 = 4 << 20

var shaRE = regexp.MustCompile("^[0-9a-f]{40}$")
var hashRE = regexp.MustCompile("^[0-9a-f]{64}$")
var uuidRE = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
var nonceRE = regexp.MustCompile("^[0-9a-f]{32}$")
var identifierRE = regexp.MustCompile("^[A-Za-z0-9_]{1,64}$")
var operationIDRE = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$")
var sourceTables = []string{"assessment", "evaluation_outcome", "interpretation_admission_failure", "interpretation_attention_projection", "assessment_score", "statistics_assessment_fact", "statistics_assessment_daily", "statistics_org_snapshot", "domain_event_outbox", "runtime_checkpoint", "retry_event_hold"}
var suffixes = []string{"20260827114947", "20260827131756"}

func targetTables() []string {
	var result []string
	for _, suffix := range suffixes {
		for _, table := range sourceTables {
			result = append(result, "cbpt_"+table+"_"+suffix)
		}
	}
	return result
}
func qi(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

type connectionConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}
type options struct{ operation, connection, defaultsFile, archiveDir, sourceSHA, marker string }
type column struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	Extra        string `json:"extra"`
	CharacterSet string `json:"character_set"`
	Collation    string `json:"collation"`
}
type tableFingerprint struct {
	ContentMeasured bool     `json:"content_measured"`
	Name            string   `json:"name"`
	DDL             string   `json:"ddl"`
	SchemaSHA256    string   `json:"schema_sha256"`
	Rows            uint64   `json:"rows"`
	ContentSHA256   string   `json:"content_sha256"`
	PK              []string `json:"pk"`
	Columns         []column `json:"columns"`
}
type nonTarget struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	DDL          string `json:"ddl"`
	SchemaSHA256 string `json:"schema_sha256"`
}
type auditRecord struct {
	FormatVersion    int                `json:"format_version"`
	OperationID      string             `json:"operation_id"`
	SourceSHA        string             `json:"source_sha"`
	SourceServerUUID string             `json:"source_server_uuid"`
	SourceTargetHash string             `json:"source_target_hash"`
	ArchiveEligible  bool               `json:"archive_eligible"`
	DropEligible     bool               `json:"drop_eligible"`
	ErrorCategory    string             `json:"error_category"`
	Tables           []tableFingerprint `json:"tables"`
	NonTargets       []nonTarget        `json:"non_targets"`
}
type manifest struct {
	FormatVersion       int                `json:"format_version"`
	OperationID         string             `json:"operation_id"`
	SourceSHA           string             `json:"source_sha"`
	SourceServerUUID    string             `json:"source_server_uuid"`
	SourceTargetHash    string             `json:"source_target_hash"`
	MigrationVersion    int                `json:"migration_version"`
	MigrationDirty      bool               `json:"migration_dirty"`
	ArchiveDropEligible bool               `json:"archive_drop_eligible"`
	Tables              []tableFingerprint `json:"tables"`
	NonTargets          []nonTarget        `json:"non_targets"`
	DumpFile            string             `json:"dump_file"`
	DumpSHA256          string             `json:"dump_sha256"`
}
type restoreMarker struct {
	FormatVersion     int    `json:"format_version"`
	OperationID       string `json:"operation_id"`
	Nonce             string `json:"nonce"`
	ContainerOwner    string `json:"container_owner"`
	SourceServerUUID  string `json:"source_server_uuid"`
	RestoreServerUUID string `json:"restore_server_uuid"`
	RestoreDatabase   string `json:"restore_database"`
	SourceTargetHash  string `json:"source_target_hash"`
}
type restoreProof struct {
	FormatVersion     int    `json:"format_version"`
	OperationID       string `json:"operation_id"`
	SourceSHA         string `json:"source_sha"`
	SourceTargetHash  string `json:"source_target_hash"`
	ManifestSHA256    string `json:"manifest_sha256"`
	DumpSHA256        string `json:"dump_sha256"`
	MarkerSHA256      string `json:"marker_sha256"`
	RestoreServerUUID string `json:"restore_server_uuid"`
	Verified          bool   `json:"verified"`
	TargetTableCount  int    `json:"target_table_count"`
}
type dropEntry struct {
	Name  string `json:"name"`
	State string `json:"state"`
}
type ledger struct {
	FormatVersion    int         `json:"format_version"`
	OperationID      string      `json:"operation_id"`
	SourceSHA        string      `json:"source_sha"`
	SourceTargetHash string      `json:"source_target_hash"`
	ManifestSHA256   string      `json:"manifest_sha256"`
	ProofSHA256      string      `json:"proof_sha256"`
	Entries          []dropEntry `json:"entries"`
	Complete         bool        `json:"complete"`
}
type receipt struct {
	FormatVersion        int    `json:"format_version"`
	Operation            string `json:"operation"`
	Stage                string `json:"stage"`
	Status               string `json:"status"`
	ErrorCategory        string `json:"error_category"`
	ErrorCode            uint16 `json:"error_code"`
	ArchiveEligible      bool   `json:"archive_eligible"`
	DropEligible         bool   `json:"drop_eligible"`
	SourceTargetHash     string `json:"source_target_hash"`
	SourceSHA            string `json:"source_sha"`
	OperationID          string `json:"operation_id"`
	TargetTableCount     int    `json:"target_table_count"`
	NonTargetCount       int    `json:"non_target_count"`
	ManifestSHA256       string `json:"manifest_sha256"`
	DumpSHA256           string `json:"dump_sha256"`
	ProofSHA256          string `json:"proof_sha256"`
	DropBlockReason      string `json:"drop_block_reason"`
	DroppedCount         int    `json:"dropped_count"`
	PendingCount         int    `json:"pending_count"`
	UnknownCount         int    `json:"unknown_count"`
	FailedCount          int    `json:"failed_count"`
	RemainingTargetCount *int   `json:"remaining_target_count,omitempty"`
	LedgerComplete       bool   `json:"ledger_complete"`
}
type operationError struct {
	category string
	cause    error
}

func (e *operationError) Error() string { return e.category }
func fail(category string, cause ...error) error {
	var e error
	if len(cause) > 0 {
		e = cause[0]
	}
	return &operationError{category: category, cause: e}
}
func errorCategory(err error) (string, uint16) {
	var e *operationError
	if !errors.As(err, &e) {
		return "internal_failure", 0
	}
	var m *mysql.MySQLError
	if errors.As(e.cause, &m) {
		return e.category, m.Number
	}
	return e.category, 0
}
func main() {
	flag.CommandLine = flag.NewFlagSet("cleanup_cbpt_tables", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	var o options
	flag.StringVar(&o.operation, "operation", "audit", "audit|archive|verify-restored|apply|verify-removed")
	flag.StringVar(&o.connection, "connection", "", "private connection JSON")
	flag.StringVar(&o.defaultsFile, "defaults-file", "", "private matching MySQL client options")
	flag.StringVar(&o.archiveDir, "archive-dir", "", "private operation directory")
	flag.StringVar(&o.sourceSHA, "source-sha", "", "reviewed source SHA")
	flag.StringVar(&o.marker, "restore-target-marker", "", "private isolated restore marker")
	parseErr := flag.CommandLine.Parse(os.Args[1:])
	if flag.NArg() != 0 {
		parseErr = errors.New("positional_arguments")
	}
	r := receipt{FormatVersion: 1, Operation: safeOperation(o.operation), Stage: safeOperation(o.operation), Status: "blocked", ErrorCategory: "none", TargetTableCount: 22, PendingCount: 22}
	if shaRE.MatchString(o.sourceSHA) {
		r.SourceSHA = o.sourceSHA
	}
	if operationIDRE.MatchString(filepath.Base(o.archiveDir)) {
		r.OperationID = filepath.Base(o.archiveDir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var err error
	if parseErr != nil {
		err = fail("invalid_options")
	} else {
		err = run(ctx, o, &r)
	}
	if err != nil {
		r.ErrorCategory, r.ErrorCode = errorCategory(err)
		if r.Status != "unknown" {
			r.Status = "blocked"
		}
	}
	// No database name, UUID, SQL, row value, password, command or raw error is emitted.
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if err != nil {
		os.Exit(1)
	}
}
func safeOperation(s string) string {
	switch s {
	case "audit", "archive", "verify-restored", "apply", "verify-removed":
		return s
	}
	return "unknown"
}
func run(ctx context.Context, o options, r *receipt) error {
	if !shaRE.MatchString(o.sourceSHA) || !operationIDRE.MatchString(filepath.Base(o.archiveDir)) || !filepath.IsAbs(o.archiveDir) {
		return fail("invalid_options")
	}
	switch o.operation {
	case "audit", "archive", "verify-restored", "apply", "verify-removed":
	default:
		return fail("invalid_operation")
	}
	if err := privateDir(o.archiveDir); err != nil {
		return err
	}
	var cfg connectionConfig
	if _, err := readPrivateJSON(o.connection, &cfg); err != nil {
		return err
	}
	if cfg.Host == "" || len(cfg.Host) > 255 || strings.ContainsAny(cfg.Host, "\x00\r\n") || cfg.Port < 1 || cfg.Port > 65535 || cfg.User == "" || len(cfg.User) > 128 || cfg.Password == "" || strings.ContainsRune(cfg.Password, 0) || !identifierRE.MatchString(cfg.Database) {
		return fail("invalid_connection")
	}
	if err := checkDefaultsFile(o.defaultsFile, cfg); err != nil {
		return err
	}
	db, conn, id, uuid, err := connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = conn.Close() }()
	targetHash := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)
	r.SourceTargetHash = targetHash
	switch o.operation {
	case "audit":
		a, err := audit(ctx, conn, cfg.Database, uuid, o, targetHash)
		if err != nil {
			return err
		}
		if err = writeAtomicJSON(filepath.Join(o.archiveDir, "audit.json"), a, true); err != nil {
			return err
		}
		r.ArchiveEligible = a.ArchiveEligible
		r.DropEligible = a.DropEligible
		r.TargetTableCount = len(a.Tables)
		r.NonTargetCount = len(a.NonTargets)
		r.DropBlockReason = a.ErrorCategory
		r.Status = "ok"
		if !a.DropEligible {
			r.Status = "archive_only"
		}
		return nil
	case "archive":
		return archive(ctx, conn, id, cfg, uuid, targetHash, o, r)
	case "verify-restored":
		return verifyRestored(ctx, conn, cfg, uuid, o, r)
	case "apply":
		return apply(ctx, conn, id, cfg, uuid, targetHash, o, r)
	case "verify-removed":
		return verifyRemoved(ctx, conn, cfg, uuid, targetHash, o, r)
	}
	return fail("invalid_operation")
}
func connect(ctx context.Context, c connectionConfig) (*sql.DB, *sql.Conn, uint64, string, error) {
	config := mysql.NewConfig()
	config.User = c.User
	config.Passwd = c.Password
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	config.DBName = c.Database
	config.Timeout = 5 * time.Second
	config.ReadTimeout = 30 * time.Second
	config.WriteTimeout = 30 * time.Second
	config.MultiStatements = false
	config.ParseTime = false
	config.Loc = time.UTC
	config.Params = map[string]string{"charset": "utf8mb4"}
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, nil, 0, "", fail("connection_failed")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	cconn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, nil, 0, "", fail("connection_failed", err)
	}
	cleanup := func() { _ = cconn.Close(); _ = db.Close() }
	for _, stmt := range []string{"SET SESSION lock_wait_timeout = 5", "SET SESSION innodb_lock_wait_timeout = 5", "SET SESSION foreign_key_checks = 1", "SET SESSION time_zone = '+00:00'"} {
		if _, err = cconn.ExecContext(ctx, stmt); err != nil {
			cleanup()
			return nil, nil, 0, "", fail("session_setup_failed", err)
		}
	}
	var id uint64
	var uuid, dbName string
	if err = cconn.QueryRowContext(ctx, "SELECT CONNECTION_ID(), @@server_uuid, DATABASE()").Scan(&id, &uuid, &dbName); err != nil || !uuidRE.MatchString(uuid) || dbName != c.Database {
		cleanup()
		return nil, nil, 0, "", fail("target_identity_failed", err)
	}
	return db, cconn, id, uuid, nil
}
func assertSession(ctx context.Context, c *sql.Conn, id uint64) error {
	var actual uint64
	if err := c.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&actual); err != nil || actual != id {
		return fail("lock_session_lost", err)
	}
	return nil
}
func migrationHead(ctx context.Context, c *sql.Conn) error {
	rows, err := c.QueryContext(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 2")
	if err != nil {
		return fail("migration_head_failed", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var version int
		var dirty bool
		if err = rows.Scan(&version, &dirty); err != nil {
			return fail("migration_head_failed", err)
		}
		n++
		if version != 95 || dirty {
			return fail("migration_head_mismatch")
		}
	}
	if rows.Err() != nil || n != 1 {
		return fail("migration_head_mismatch")
	}
	return nil
}

type object struct{ name, kind, engine string }

func objects(ctx context.Context, c *sql.Conn, dbName string) ([]object, error) {
	rows, err := c.QueryContext(ctx, "SELECT table_name, table_type, COALESCE(engine,'') FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name LIMIT 1001", dbName)
	if err != nil {
		return nil, fail("catalog_failed", err)
	}
	defer func() { _ = rows.Close() }()
	var out []object
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.name, &o.kind, &o.engine); err != nil {
			return nil, fail("catalog_failed", err)
		}
		if !identifierRE.MatchString(o.name) {
			return nil, fail("catalog_unsupported_identifier")
		}
		out = append(out, o)
		if len(out) > 1000 {
			return nil, fail("catalog_limit")
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fail("catalog_failed", err)
	}
	return out, nil
}
func audit(ctx context.Context, c *sql.Conn, dbName, uuid string, o options, targetHash string) (auditRecord, error) {
	a := auditRecord{FormatVersion: 1, OperationID: filepath.Base(o.archiveDir), SourceSHA: o.sourceSHA, SourceServerUUID: uuid, SourceTargetHash: targetHash, ErrorCategory: "none"}
	if err := migrationHead(ctx, c); err != nil {
		return a, err
	}
	obs, err := objects(ctx, c, dbName)
	if err != nil {
		return a, err
	}
	targets := map[string]bool{}
	sources := map[string]bool{}
	for _, t := range targetTables() {
		targets[t] = true
	}
	for _, t := range sourceTables {
		sources[t] = true
	}
	for _, obj := range obs {
		if targets[obj.name] {
			if obj.kind != "BASE TABLE" || obj.engine != "InnoDB" {
				return a, fail("target_table_type_blocked")
			}
			fp, err := fingerprint(ctx, c, dbName, obj.name, false)
			if err != nil {
				return a, err
			}
			a.Tables = append(a.Tables, fp)
		} else {
			ddl, err := tableDDL(ctx, c, dbName, obj.name)
			if err != nil {
				return a, err
			}
			a.NonTargets = append(a.NonTargets, nonTarget{Name: obj.name, Kind: obj.kind, DDL: ddl, SchemaSHA256: hashText(canonicalDDL(ddl))})
			if sources[obj.name] && obj.kind != "BASE TABLE" {
				return a, fail("source_table_type_blocked")
			}
			delete(sources, obj.name)
		}
	}
	if len(a.Tables) != 22 || len(a.NonTargets) != 66 || len(sources) != 0 {
		return a, fail("object_set_mismatch")
	}
	a.ArchiveEligible = true
	err = dependencies(ctx, c, dbName)
	if err != nil {
		a.ErrorCategory, _ = errorCategory(err)
		return a, nil
	}
	a.DropEligible = true
	return a, nil
}
func tableDDL(ctx context.Context, c *sql.Conn, dbName, table string) (string, error) {
	rows, err := c.QueryContext(ctx, "SHOW CREATE TABLE "+qi(dbName)+"."+qi(table))
	if err != nil {
		return "", fail("schema_definition_failed", err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil || len(cols) < 2 || len(cols) > 4 {
		return "", fail("schema_definition_failed")
	}
	if !rows.Next() {
		return "", fail("schema_definition_failed")
	}
	vals := make([]sql.RawBytes, len(cols))
	args := make([]any, len(cols))
	for i := range args {
		args[i] = &vals[i]
	}
	if err = rows.Scan(args...); err != nil || vals[1] == nil || len(vals[1]) > 1<<20 {
		return "", fail("schema_definition_failed", err)
	}
	ddl := string(vals[1])
	if string(vals[0]) != table || ddl == "" {
		return "", fail("schema_definition_failed")
	}
	if rows.Next() || rows.Err() != nil {
		return "", fail("schema_definition_failed")
	}
	return ddl, nil
}

// Only outside quoted SQL literals/identifiers: remove the mutable next identity value.
// All expressions, defaults, comments, keys and collations remain in the schema hash.
func canonicalDDL(s string) string {
	var out strings.Builder
	quote := byte(0)
	depth := 0
	for i := 0; i < len(s); {
		ch := s[i]
		if quote != 0 {
			out.WriteByte(ch)
			i++
			if ch == '\\' && i < len(s) {
				out.WriteByte(s[i])
				i++
				continue
			}
			if ch == quote {
				if i < len(s) && s[i] == quote {
					out.WriteByte(s[i])
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		// Preserve comments as written, including parentheses and keyword text.
		if strings.HasPrefix(s[i:], "/*") {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				out.WriteString(s[i:])
				break
			}
			end += i + 4
			out.WriteString(s[i:end])
			i = end
			continue
		}
		if ch == '#' || (strings.HasPrefix(s[i:], "--") && i+2 < len(s) && ddlSpace(s[i+2])) {
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				out.WriteString(s[i:])
				break
			}
			end += i
			out.WriteString(s[i:end])
			i = end
			continue
		}
		if depth == 1 && (i == 0 || ddlSpace(s[i-1])) {
			if end := redundantColumnCharsetEnd(s, i); end > i {
				i = end
				continue
			}
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			out.WriteByte(ch)
			i++
			continue
		}
		if (i == 0 || s[i-1] == ' ') && len(s)-i >= len("AUTO_INCREMENT=") && strings.EqualFold(s[i:i+len("AUTO_INCREMENT=")], "AUTO_INCREMENT=") {
			j := i + len("AUTO_INCREMENT=")
			k := j
			for k < len(s) && s[k] >= '0' && s[k] <= '9' {
				k++
			}
			if k > j && (k == len(s) || s[k] == ' ' || s[k] == '\n' || s[k] == '\r') {
				i = k
				continue
			}
		}
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			if out.Len() > 0 && !strings.HasSuffix(out.String(), " ") {
				out.WriteByte(' ')
			}
			i++
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}
		out.WriteByte(ch)
		i++
	}
	return strings.TrimSpace(out.String())
}

func ddlSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// MySQL bug #110825 changes SHOW CREATE presentation after dump/recreation.
// At column-definition depth only, COLLATE already identifies its character
// set, so the adjacent, matching CHARACTER SET is redundant. Expressions,
// quoted text, comments, and every COLLATE clause remain unchanged.
func redundantColumnCharsetEnd(s string, at int) int {
	pos := at
	word := func() string {
		start := pos
		for pos < len(s) && ((s[pos] >= 'a' && s[pos] <= 'z') || (s[pos] >= 'A' && s[pos] <= 'Z') || (s[pos] >= '0' && s[pos] <= '9') || s[pos] == '_') {
			pos++
		}
		return s[start:pos]
	}
	space := func() bool {
		start := pos
		for pos < len(s) && ddlSpace(s[pos]) {
			pos++
		}
		return pos > start
	}
	if !strings.EqualFold(word(), "CHARACTER") || !space() || !strings.EqualFold(word(), "SET") || !space() {
		return at
	}
	charset := word()
	if charset == "" || !space() {
		return at
	}
	collateAt := pos
	if !strings.EqualFold(word(), "COLLATE") || !space() {
		return at
	}
	collation := word()
	if !strings.HasPrefix(strings.ToLower(collation), strings.ToLower(charset)+"_") {
		return at
	}
	return collateAt
}

func fingerprint(ctx context.Context, c *sql.Conn, dbName, table string, content bool) (tableFingerprint, error) {
	fp := tableFingerprint{Name: table}
	ddl, err := tableDDL(ctx, c, dbName, table)
	if err != nil {
		return fp, err
	}
	fp.DDL = ddl
	fp.SchemaSHA256 = hashText(canonicalDDL(ddl))
	rows, err := c.QueryContext(ctx, "SELECT column_name, data_type, extra, COALESCE(character_set_name,''), COALESCE(collation_name,'') FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position LIMIT 257", dbName, table)
	if err != nil {
		return fp, fail("column_definition_failed", err)
	}
	for rows.Next() {
		var col column
		if err = rows.Scan(&col.Name, &col.DataType, &col.Extra, &col.CharacterSet, &col.Collation); err != nil {
			_ = rows.Close()
			return fp, fail("column_definition_failed", err)
		}
		if !identifierRE.MatchString(col.Name) {
			_ = rows.Close()
			return fp, fail("column_identifier_blocked")
		}
		fp.Columns = append(fp.Columns, col)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(fp.Columns) == 0 || len(fp.Columns) > 256 {
		return fp, fail("column_definition_failed", err)
	}
	rows, err = c.QueryContext(ctx, "SELECT column_name FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name='PRIMARY' ORDER BY seq_in_index LIMIT 257", dbName, table)
	if err != nil {
		return fp, fail("primary_key_failed", err)
	}
	for rows.Next() {
		var name sql.NullString
		if err = rows.Scan(&name); err != nil || !name.Valid || !identifierRE.MatchString(name.String) {
			_ = rows.Close()
			return fp, fail("primary_key_failed", err)
		}
		fp.PK = append(fp.PK, name.String)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(fp.PK) == 0 || len(fp.PK) > 256 {
		return fp, fail("primary_key_required", err)
	}
	if !content {
		return fp, nil
	}
	fp.ContentMeasured = true
	names := make([]string, len(fp.Columns))
	for i, col := range fp.Columns {
		names[i] = qi(col.Name)
	}
	pk := make([]string, len(fp.PK))
	for i, col := range fp.PK {
		pk[i] = qi(col)
	}
	rows, err = c.QueryContext(ctx, "SELECT "+strings.Join(names, ",")+" FROM "+qi(dbName)+"."+qi(table)+" ORDER BY "+strings.Join(pk, ","))
	if err != nil {
		return fp, fail("content_read_failed", err)
	}
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	writeUint(h, uint64(len(names)))
	vals := make([]sql.RawBytes, len(names))
	args := make([]any, len(names))
	for i := range args {
		args[i] = &vals[i]
	}
	for rows.Next() {
		if err = rows.Scan(args...); err != nil {
			return fp, fail("content_read_failed", err)
		}
		for _, v := range vals {
			if len(v) > 16<<20 {
				return fp, fail("content_value_limit")
			}
			writeCell(h, v)
		}
		fp.Rows++
	}
	if err = rows.Err(); err != nil {
		return fp, fail("content_read_failed", err)
	}
	writeUint(h, fp.Rows)
	fp.ContentSHA256 = hex.EncodeToString(h.Sum(nil))
	return fp, nil
}
func writeUint(w io.Writer, n uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	_, _ = w.Write(b[:])
}
func writeCell(w io.Writer, v []byte) {
	if v == nil {
		_, _ = w.Write([]byte{0})
		return
	}
	_, _ = w.Write([]byte{1})
	writeUint(w, uint64(len(v)))
	_, _ = w.Write(v)
}
func hashText(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		writeCell(h, []byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func dependencies(ctx context.Context, c *sql.Conn, dbName string) error {
	rows, err := c.QueryContext(ctx, "SHOW GRANTS FOR CURRENT_USER")
	if err != nil {
		return fail("metadata_visibility_blocked", err)
	}
	statements := make([]string, 0, 10)
	n := 0
	for rows.Next() {
		var g string
		if err = rows.Scan(&g); err != nil {
			_ = rows.Close()
			return fail("metadata_visibility_blocked", err)
		}
		n++
		if n > 100 {
			_ = rows.Close()
			return fail("metadata_limit")
		}
		statements = append(statements, g)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return fail("metadata_visibility_blocked", err)
	}
	if !metadataGrantsVisible(statements) {
		return fail("metadata_visibility_blocked")
	}
	rows, err = c.QueryContext(ctx, "SELECT table_schema,table_name,referenced_table_schema,referenced_table_name FROM information_schema.key_column_usage WHERE referenced_table_name IS NOT NULL LIMIT 10001")
	if err != nil {
		return fail("dependency_read_failed", err)
	}
	targets := map[string]bool{}
	for _, name := range targetTables() {
		targets[name] = true
	}
	n = 0
	for rows.Next() {
		var s, t, rs, rt string
		if err = rows.Scan(&s, &t, &rs, &rt); err != nil {
			_ = rows.Close()
			return fail("dependency_read_failed", err)
		}
		n++
		if n > 10000 {
			_ = rows.Close()
			return fail("metadata_limit")
		}
		if strings.EqualFold(s, dbName) && targets[strings.ToLower(t)] || strings.EqualFold(rs, dbName) && targets[strings.ToLower(rt)] {
			_ = rows.Close()
			return fail("foreign_key_dependency")
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return fail("dependency_read_failed", err)
	}
	queries := []string{
		"SELECT event_object_schema,event_object_table,action_statement FROM information_schema.triggers LIMIT 10001",
		"SELECT table_schema,table_name,view_definition FROM information_schema.views LIMIT 10001",
		"SELECT routine_schema,routine_name,routine_definition FROM information_schema.routines LIMIT 10001",
		"SELECT event_schema,event_name,event_definition FROM information_schema.events LIMIT 10001",
	}
	dynamic := regexp.MustCompile(`(?i)\b(prepare|execute|execute_prepared_stmt)\b`)
	total := 0
	for i, q := range queries {
		rows, err = c.QueryContext(ctx, q)
		if err != nil {
			return fail("dependency_read_failed", err)
		}
		n = 0
		for rows.Next() {
			var s, name string
			var body sql.NullString
			if err = rows.Scan(&s, &name, &body); err != nil {
				_ = rows.Close()
				return fail("dependency_read_failed", err)
			}
			n++
			total += len(body.String)
			if n > 10000 || total > 16<<20 || len(body.String) > 1<<20 {
				_ = rows.Close()
				return fail("metadata_limit")
			}
			if !body.Valid || body.String == "" {
				_ = rows.Close()
				return fail("metadata_definition_hidden")
			}
			if i == 0 && strings.EqualFold(s, dbName) && targets[strings.ToLower(name)] {
				_ = rows.Close()
				return fail("trigger_dependency")
			}
			if strings.Contains(strings.ToLower(body.String), "cbpt_") {
				_ = rows.Close()
				return fail("definition_dependency")
			}
			system := s == "mysql" || s == "sys" || s == "information_schema" || s == "performance_schema"
			if !system && i >= 2 && dynamic.MatchString(body.String) {
				_ = rows.Close()
				return fail("dynamic_definition_unknown")
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fail("dependency_read_failed", err)
		}
	}
	return nil
}

// SHOW GRANTS can contain schema-level partial revocations even alongside a
// global privilege grant. Such restrictions make empty cross-schema metadata
// inconclusive; only positively proven unrestricted global visibility is used.
func metadataGrantsVisible(statements []string) bool {
	grants := map[string]bool{}
	for _, statement := range statements {
		upper := strings.ToUpper(strings.TrimSpace(statement))
		if !strings.HasPrefix(upper, "GRANT ") {
			return false
		}
		at := strings.Index(upper, " ON *.* TO ")
		if at > 6 {
			for _, privilege := range strings.Split(upper[6:at], ",") {
				grants[strings.TrimSpace(privilege)] = true
			}
		}
	}
	if grants["ALL PRIVILEGES"] {
		return true
	}
	for _, privilege := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT"} {
		if !grants[privilege] {
			return false
		}
	}
	return true
}

func lockSQL(dbName, mode string) string {
	parts := make([]string, 0, 22)
	for _, t := range targetTables() {
		parts = append(parts, qi(dbName)+"."+qi(t)+" "+mode)
	}
	return "LOCK TABLES " + strings.Join(parts, ",")
}
func takeLock(ctx context.Context, c *sql.Conn, id uint64, dbName, mode string) error {
	if err := assertSession(ctx, c, id); err != nil {
		return err
	}
	if _, err := c.ExecContext(ctx, lockSQL(dbName, mode)); err != nil {
		return fail("target_lock_failed", err)
	}
	return assertSession(ctx, c, id)
}
func unlock(c *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = c.ExecContext(ctx, "UNLOCK TABLES")
}
func archive(ctx context.Context, c *sql.Conn, id uint64, cfg connectionConfig, uuid, targetHash string, o options, r *receipt) error {
	for _, f := range []string{"manifest.json", "dump.sql.gz", "dump.sql.gz.partial", "restore-proof.json", "drop-ledger.json"} {
		if _, err := os.Lstat(filepath.Join(o.archiveDir, f)); !os.IsNotExist(err) {
			return fail("archive_already_exists")
		}
	}
	a, err := audit(ctx, c, cfg.Database, uuid, o, targetHash)
	if err != nil {
		return err
	}
	if err = takeLock(ctx, c, id, cfg.Database, "READ"); err != nil {
		return err
	}
	defer unlock(c)
	m := manifest{FormatVersion: 1, OperationID: a.OperationID, SourceSHA: o.sourceSHA, SourceServerUUID: uuid, SourceTargetHash: targetHash, MigrationVersion: 95, ArchiveDropEligible: a.DropEligible, NonTargets: a.NonTargets, DumpFile: "dump.sql.gz"}
	for _, t := range targetTables() {
		if err = assertSession(ctx, c, id); err != nil {
			return err
		}
		fp, e := fingerprint(ctx, c, cfg.Database, t, true)
		if e != nil {
			return e
		}
		m.Tables = append(m.Tables, fp)
	}
	if err = assertSession(ctx, c, id); err != nil {
		return err
	}
	if err = runDump(ctx, o, cfg.Database); err != nil {
		return err
	}
	if err = assertSession(ctx, c, id); err != nil {
		return err
	}
	m.DumpSHA256, err = fileHash(filepath.Join(o.archiveDir, m.DumpFile), maxDumpBytes)
	if err != nil {
		return err
	}
	if err = writeAtomicJSON(filepath.Join(o.archiveDir, "manifest.json"), m, false); err != nil {
		return err
	}
	r.ManifestSHA256, err = fileHash(filepath.Join(o.archiveDir, "manifest.json"), maxJSONBytes)
	if err != nil {
		return err
	}
	r.DumpSHA256 = m.DumpSHA256
	r.TargetTableCount = 22
	r.NonTargetCount = 66
	r.ArchiveEligible = true
	r.DropEligible = a.DropEligible
	r.DropBlockReason = a.ErrorCategory
	r.Status = "ok"
	if !a.DropEligible {
		r.Status = "archive_only"
	}
	return nil
}

type cappedWriter struct {
	w     io.Writer
	n     int64
	limit int64
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.limit-w.n {
		return 0, fail("dump_size_limit")
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}
func dumpArgs(o options, dbName string) []string {
	args := []string{"--defaults-file=" + o.defaultsFile, "--single-transaction", "--quick", "--set-gtid-purged=OFF", "--no-tablespaces", "--skip-triggers", "--skip-routines", "--skip-events", "--hex-blob", "--default-character-set=utf8mb4", "--skip-comments", "--skip-add-locks", "--skip-add-drop-table", "--skip-lock-tables", dbName}
	return append(args, targetTables()...)
}
func runDump(ctx context.Context, o options, dbName string) error {
	part := filepath.Join(o.archiveDir, "dump.sql.gz.partial")
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail("dump_file_failed")
	}
	defer func() { _ = f.Close() }()
	cw := &cappedWriter{w: f, limit: maxDumpBytes}
	gz := gzip.NewWriter(cw)
	home, err := os.MkdirTemp("", "cbpt-dump-home-")
	if err != nil {
		return fail("dump_environment_failed")
	}
	defer func() { _ = os.RemoveAll(home) }()
	command := exec.CommandContext(ctx, "mysqldump", dumpArgs(o, dbName)...)
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if key != "HOME" && key != "MYSQL_PWD" && key != "MYSQL_HOST" && key != "MYSQL_TCP_PORT" && key != "MYSQL_UNIX_PORT" && key != "MYSQL_TEST_LOGIN_FILE" {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env, "HOME="+home, "MYSQL_TEST_LOGIN_FILE="+filepath.Join(home, "nonexistent-login-file"))
	command.Stdout = gz
	command.Stderr = io.Discard
	command.WaitDelay = 5 * time.Second
	if err = command.Run(); err != nil {
		_ = gz.Close()
		return fail("dump_client_failed")
	}
	if err = gz.Close(); err != nil {
		return fail("dump_write_failed")
	}
	if err = f.Sync(); err != nil {
		return fail("dump_fsync_failed")
	}
	if err = f.Close(); err != nil {
		return fail("dump_close_failed")
	}
	if err = os.Rename(part, filepath.Join(o.archiveDir, "dump.sql.gz")); err != nil {
		return fail("dump_publish_failed")
	}
	return syncDir(o.archiveDir)
}
func loadManifest(o options) (manifest, string, error) {
	var m manifest
	b, err := readPrivateJSON(filepath.Join(o.archiveDir, "manifest.json"), &m)
	if err != nil {
		return m, "", err
	}
	if err = validateManifest(m, o); err != nil {
		return m, "", err
	}
	actual, err := fileHash(filepath.Join(o.archiveDir, "dump.sql.gz"), maxDumpBytes)
	if err != nil || actual != m.DumpSHA256 {
		return m, "", fail("archive_dump_mismatch", err)
	}
	sum := sha256.Sum256(b)
	return m, hex.EncodeToString(sum[:]), nil
}
func validateManifest(m manifest, o options) error {
	if m.FormatVersion != 1 || m.OperationID != filepath.Base(o.archiveDir) || m.SourceSHA != o.sourceSHA || !uuidRE.MatchString(m.SourceServerUUID) || !hashRE.MatchString(m.SourceTargetHash) || m.MigrationVersion != 95 || m.MigrationDirty || m.DumpFile != "dump.sql.gz" || !hashRE.MatchString(m.DumpSHA256) {
		return fail("manifest_invalid")
	}
	targets := map[string]bool{}
	for _, t := range targetTables() {
		targets[t] = true
	}
	if len(m.Tables) != 22 || len(m.NonTargets) != 66 {
		return fail("manifest_object_set_invalid")
	}
	for _, t := range m.Tables {
		if !t.ContentMeasured || !targets[t.Name] || !hashRE.MatchString(t.SchemaSHA256) || !hashRE.MatchString(t.ContentSHA256) || len(t.PK) == 0 || len(t.Columns) == 0 || hashText(canonicalDDL(t.DDL)) != t.SchemaSHA256 {
			return fail("manifest_table_invalid")
		}
		delete(targets, t.Name)
		cols := map[string]bool{}
		for _, col := range t.Columns {
			if !identifierRE.MatchString(col.Name) || cols[col.Name] {
				return fail("manifest_column_invalid")
			}
			cols[col.Name] = true
		}
		for _, pk := range t.PK {
			if !cols[pk] {
				return fail("manifest_pk_invalid")
			}
		}
	}
	seen := map[string]bool{}
	sources := map[string]bool{}
	for _, t := range sourceTables {
		sources[t] = true
	}
	for _, t := range m.NonTargets {
		if !identifierRE.MatchString(t.Name) || seen[t.Name] || !hashRE.MatchString(t.SchemaSHA256) || hashText(canonicalDDL(t.DDL)) != t.SchemaSHA256 {
			return fail("manifest_non_target_invalid")
		}
		seen[t.Name] = true
		delete(sources, t.Name)
		for _, name := range targetTables() {
			if name == t.Name {
				return fail("manifest_non_target_invalid")
			}
		}
	}
	if len(sources) != 0 {
		return fail("manifest_source_missing")
	}
	return nil
}
func sameTable(a, b tableFingerprint) bool {
	x, _ := json.Marshal(a.PK)
	y, _ := json.Marshal(b.PK)
	xc, _ := json.Marshal(a.Columns)
	yc, _ := json.Marshal(b.Columns)
	return a.ContentMeasured == b.ContentMeasured && a.Name == b.Name && a.SchemaSHA256 == b.SchemaSHA256 && a.Rows == b.Rows && a.ContentSHA256 == b.ContentSHA256 && string(x) == string(y) && string(xc) == string(yc)
}
func verifyRestored(ctx context.Context, c *sql.Conn, cfg connectionConfig, uuid string, o options, r *receipt) error {
	m, mh, err := loadManifest(o)
	if err != nil {
		return err
	}
	var marker restoreMarker
	b, err := readPrivateJSON(o.marker, &marker)
	if err != nil {
		return err
	}
	if marker.FormatVersion != 1 || marker.OperationID != m.OperationID || !nonceRE.MatchString(marker.Nonce) || !nonceRE.MatchString(marker.ContainerOwner) || marker.SourceServerUUID != m.SourceServerUUID || marker.SourceTargetHash != m.SourceTargetHash || marker.RestoreServerUUID != uuid || !uuidRE.MatchString(uuid) || uuid == m.SourceServerUUID || marker.RestoreDatabase != "cbpt_restore_"+marker.Nonce || cfg.Database != marker.RestoreDatabase {
		return fail("restore_target_marker_invalid")
	}
	obs, err := objects(ctx, c, cfg.Database)
	if err != nil {
		return err
	}
	if len(obs) != 22 {
		return fail("restored_object_set_mismatch")
	}
	for _, expected := range m.Tables {
		for _, obj := range obs {
			if obj.name == expected.Name && (obj.kind != "BASE TABLE" || obj.engine != "InnoDB") {
				return fail("restored_table_type_blocked")
			}
		}
		fp, e := fingerprint(ctx, c, cfg.Database, expected.Name, true)
		if e != nil {
			return e
		}
		if !sameTable(expected, fp) {
			return fail("restored_content_mismatch")
		}
	}
	markerSum := sha256.Sum256(b)
	proof := restoreProof{FormatVersion: 1, OperationID: m.OperationID, SourceSHA: o.sourceSHA, SourceTargetHash: m.SourceTargetHash, ManifestSHA256: mh, DumpSHA256: m.DumpSHA256, MarkerSHA256: hex.EncodeToString(markerSum[:]), RestoreServerUUID: uuid, Verified: true, TargetTableCount: 22}
	if err = writeAtomicJSON(filepath.Join(o.archiveDir, "restore-proof.json"), proof, false); err != nil {
		return err
	}
	r.ProofSHA256, err = fileHash(filepath.Join(o.archiveDir, "restore-proof.json"), maxJSONBytes)
	if err != nil {
		return err
	}
	r.ManifestSHA256 = mh
	r.DumpSHA256 = m.DumpSHA256
	r.SourceTargetHash = m.SourceTargetHash
	r.TargetTableCount = 22
	r.NonTargetCount = 66
	r.ArchiveEligible = true
	r.Status = "ok"
	return nil
}
func loadProof(o options, m manifest, mh string) (restoreProof, string, error) {
	var p restoreProof
	b, err := readPrivateJSON(filepath.Join(o.archiveDir, "restore-proof.json"), &p)
	if err != nil {
		return p, "", err
	}
	if p.FormatVersion != 1 || p.OperationID != m.OperationID || p.SourceSHA != m.SourceSHA || p.SourceTargetHash != m.SourceTargetHash || p.ManifestSHA256 != mh || p.DumpSHA256 != m.DumpSHA256 || !hashRE.MatchString(p.MarkerSHA256) || !uuidRE.MatchString(p.RestoreServerUUID) || p.RestoreServerUUID == m.SourceServerUUID || !p.Verified || p.TargetTableCount != 22 {
		return p, "", fail("restore_proof_invalid")
	}
	sum := sha256.Sum256(b)
	return p, hex.EncodeToString(sum[:]), nil
}
func sameNonTargets(a, b []nonTarget) bool {
	if len(a) != len(b) {
		return false
	}
	byName := map[string]nonTarget{}
	for _, t := range a {
		byName[t.Name] = t
	}
	for _, t := range b {
		old, ok := byName[t.Name]
		if !ok || old.Kind != t.Kind || old.SchemaSHA256 != t.SchemaSHA256 {
			return false
		}
	}
	return true
}
func apply(ctx context.Context, c *sql.Conn, id uint64, cfg connectionConfig, uuid, targetHash string, o options, r *receipt) error {
	m, mh, err := loadManifest(o)
	if err != nil {
		return err
	}
	_, ph, err := loadProof(o, m, mh)
	if err != nil {
		return err
	}
	if uuid != m.SourceServerUUID || targetHash != m.SourceTargetHash {
		return fail("source_identity_mismatch")
	}
	if _, err = os.Lstat(filepath.Join(o.archiveDir, "drop-ledger.json")); !os.IsNotExist(err) {
		return fail("drop_ledger_already_exists")
	}
	a, err := audit(ctx, c, cfg.Database, uuid, o, targetHash)
	if err != nil {
		return err
	}
	if !a.DropEligible {
		return fail(a.ErrorCategory)
	}
	if !sameNonTargets(a.NonTargets, m.NonTargets) {
		return fail("non_target_schema_changed")
	}
	if err = takeLock(ctx, c, id, cfg.Database, "WRITE"); err != nil {
		return err
	}
	defer unlock(c)
	// No business table/schema_migrations access occurs while only the 22 target tables are locked.
	if err = dependencies(ctx, c, cfg.Database); err != nil {
		return err
	}
	for _, expected := range m.Tables {
		if err = assertSession(ctx, c, id); err != nil {
			return err
		}
		fp, e := fingerprint(ctx, c, cfg.Database, expected.Name, true)
		if e != nil {
			return e
		}
		if !sameTable(expected, fp) {
			return fail("source_content_changed")
		}
	}
	l := ledger{FormatVersion: 1, OperationID: m.OperationID, SourceSHA: o.sourceSHA, SourceTargetHash: targetHash, ManifestSHA256: mh, ProofSHA256: ph}
	for _, name := range targetTables() {
		l.Entries = append(l.Entries, dropEntry{Name: name, State: "pending"})
	}
	if err = writeAtomicJSON(filepath.Join(o.archiveDir, "drop-ledger.json"), l, false); err != nil {
		return err
	}
	save := func(value ledger) error {
		return writeAtomicJSON(filepath.Join(o.archiveDir, "drop-ledger.json"), value, true)
	}
	drop := func(name string) error {
		if e := assertSession(ctx, c, id); e != nil {
			return e
		}
		if _, e := c.ExecContext(ctx, "DROP TABLE "+qi(cfg.Database)+"."+qi(name)); e != nil {
			return fail("drop_execution_unknown", e)
		}
		return nil
	}
	if err = dropExact(&l, save, drop); err != nil {
		receiptLedger(r, l)
		r.Status = "unknown"
		return err
	}
	receiptLedger(r, l)
	r.ManifestSHA256 = mh
	r.DumpSHA256 = m.DumpSHA256
	r.ProofSHA256 = ph
	r.TargetTableCount = 22
	r.NonTargetCount = 66
	r.ArchiveEligible = true
	r.DropEligible = true
	r.Status = "ok"
	return nil
}

// Never retry DDL: both lost responses and failed durable acknowledgement stop the sequence.
func dropExact(l *ledger, save func(ledger) error, drop func(string) error) error {
	for i := range l.Entries {
		if l.Entries[i].State != "pending" {
			return fail("drop_ledger_state_invalid")
		}
		l.Entries[i].State = "drop_started"
		if err := save(*l); err != nil {
			return fail("drop_intent_fsync_failed")
		}
		if err := drop(l.Entries[i].Name); err != nil {
			l.Entries[i].State = "unknown"
			_ = save(*l)
			return err
		}
		l.Entries[i].State = "dropped"
		if err := save(*l); err != nil {
			l.Entries[i].State = "unknown"
			l.Complete = false
			return fail("drop_ack_fsync_unknown")
		}
	}
	l.Complete = true
	if err := save(*l); err != nil {
		l.Complete = false
		return fail("drop_complete_fsync_unknown")
	}
	return nil
}
func receiptLedger(r *receipt, l ledger) {
	r.DroppedCount = 0
	r.PendingCount = 0
	r.UnknownCount = 0
	r.FailedCount = 0
	r.LedgerComplete = l.Complete
	for _, e := range l.Entries {
		switch e.State {
		case "pending":
			r.PendingCount++
		case "dropped":
			r.DroppedCount++
		case "unknown", "drop_started":
			r.UnknownCount++
		default:
			r.FailedCount++
		}
	}
}
func verifyRemoved(ctx context.Context, c *sql.Conn, cfg connectionConfig, uuid, targetHash string, o options, r *receipt) error {
	m, mh, err := loadManifest(o)
	if err != nil {
		return err
	}
	_, ph, err := loadProof(o, m, mh)
	if err != nil {
		return err
	}
	if uuid != m.SourceServerUUID || targetHash != m.SourceTargetHash {
		return fail("source_identity_mismatch")
	}
	var l ledger
	if _, err = readPrivateJSON(filepath.Join(o.archiveDir, "drop-ledger.json"), &l); err != nil {
		return err
	}
	if l.FormatVersion != 1 || l.OperationID != m.OperationID || l.SourceSHA != m.SourceSHA || l.SourceTargetHash != targetHash || l.ManifestSHA256 != mh || l.ProofSHA256 != ph || len(l.Entries) != 22 {
		return fail("drop_ledger_invalid")
	}
	expected := targetTables()
	for i, e := range l.Entries {
		if e.Name != expected[i] || (e.State != "pending" && e.State != "drop_started" && e.State != "dropped" && e.State != "unknown") {
			return fail("drop_ledger_invalid")
		}
	}
	if err = migrationHead(ctx, c); err != nil {
		return err
	}
	obs, err := objects(ctx, c, cfg.Database)
	if err != nil {
		return err
	}
	if len(obs) != 66 {
		return fail("removed_object_set_mismatch")
	}
	targets := map[string]bool{}
	for _, name := range targetTables() {
		targets[name] = true
	}
	var preserved []nonTarget
	for _, obj := range obs {
		if targets[obj.name] {
			return fail("target_still_present")
		}
		ddl, e := tableDDL(ctx, c, cfg.Database, obj.name)
		if e != nil {
			return e
		}
		preserved = append(preserved, nonTarget{Name: obj.name, Kind: obj.kind, DDL: ddl, SchemaSHA256: hashText(canonicalDDL(ddl))})
	}
	if !sameNonTargets(m.NonTargets, preserved) {
		return fail("non_target_schema_changed")
	}
	// Verification does not resume or reinterpret an uncertain DDL execution as a confirmed apply.
	r.ManifestSHA256 = mh
	r.DumpSHA256 = m.DumpSHA256
	r.ProofSHA256 = ph
	r.TargetTableCount = 22
	zero := 0
	r.RemainingTargetCount = &zero
	receiptLedger(r, l)
	if !l.Complete || r.DroppedCount != 22 || r.UnknownCount != 0 || r.PendingCount != 0 || r.FailedCount != 0 {
		r.Status = "unknown"
		return fail("drop_ledger_unconfirmed")
	}
	r.NonTargetCount = 66
	r.ArchiveEligible = true
	r.DropEligible = true
	r.Status = "ok"
	return nil
}
func privateDir(path string) error {
	if !filepath.IsAbs(path) {
		return fail("private_path_invalid")
	}
	if err := noSymlink(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return fail("private_directory_failed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fail("private_directory_permissions")
	}
	return nil
}
func noSymlink(path string) error {
	if !filepath.IsAbs(path) {
		return fail("private_path_invalid")
	}
	for p := filepath.Clean(path); p != "/"; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return fail("private_path_invalid")
		}
	}
	return nil
}
func privateFile(path string) (*os.File, error) {
	if err := noSymlink(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fail("private_file_permissions")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fail("private_file_read_failed")
	}
	return f, nil
}
func readPrivateJSON(path string, dst any) ([]byte, error) {
	f, err := privateFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxJSONBytes+1))
	if err != nil || int64(len(b)) > maxJSONBytes {
		return nil, fail("private_json_limit")
	}
	if err = exactJSONFields(b, reflect.TypeOf(dst).Elem()); err != nil {
		return nil, fail("private_json_invalid")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(dst); err != nil {
		return nil, fail("private_json_invalid")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, fail("private_json_invalid")
	}
	return b, nil
}
func writeAtomicJSON(path string, value any, replace bool) error {
	if err := privateDir(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil && !replace {
		return fail("artifact_already_exists")
	} else if err != nil && !os.IsNotExist(err) {
		return fail("artifact_path_failed")
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil || int64(len(b)) > maxJSONBytes {
		return fail("artifact_encode_failed")
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".cbpt-artifact-")
	if err != nil {
		return fail("artifact_write_failed")
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	defer func() { _ = f.Close() }()
	if err = f.Chmod(0600); err != nil {
		return fail("artifact_permissions_failed")
	}
	if _, err = f.Write(b); err != nil {
		return fail("artifact_write_failed")
	}
	if err = f.Sync(); err != nil {
		return fail("artifact_fsync_failed")
	}
	if err = f.Close(); err != nil {
		return fail("artifact_close_failed")
	}
	if !replace {
		if _, err = os.Lstat(path); !os.IsNotExist(err) {
			return fail("artifact_already_exists")
		}
	}
	if err = os.Rename(name, path); err != nil {
		return fail("artifact_publish_failed")
	}
	return syncDir(filepath.Dir(path))
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fail("directory_fsync_failed")
	}
	defer func() { _ = f.Close() }()
	if err = f.Sync(); err != nil {
		return fail("directory_fsync_failed")
	}
	return nil
}
func fileHash(path string, limit int64) (string, error) {
	f, err := privateFile(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, limit+1))
	if err != nil || n > limit {
		return "", fail("artifact_hash_failed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func checkDefaultsFile(path string, c connectionConfig) error {
	f, err := privateFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil || len(b) > 32768 {
		return fail("defaults_file_invalid")
	}
	options := map[string]string{}
	section := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[client]" && !section {
			section = true
			continue
		}
		if !section {
			return fail("defaults_file_invalid")
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || (key != "host" && key != "port" && key != "user" && key != "password") || hasKey(options, key) {
			return fail("defaults_file_invalid")
		}
		value, err = decodeOption(value)
		if err != nil {
			return err
		}
		options[key] = value
	}
	if len(options) != 4 || options["host"] != c.Host || options["port"] != strconv.Itoa(c.Port) || options["user"] != c.User || options["password"] != c.Password {
		return fail("defaults_file_mismatch")
	}
	return nil
}
func hasKey(m map[string]string, k string) bool { _, ok := m[k]; return ok }
func decodeOption(s string) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fail("defaults_file_invalid")
	}
	s = s[1 : len(s)-1]
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			return "", fail("defaults_file_invalid")
		}
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i == len(s) {
			return "", fail("defaults_file_invalid")
		}
		switch s[i] {
		case '\\', '"', '\'':
			out.WriteByte(s[i])
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 's':
			out.WriteByte(' ')
		default:
			return "", fail("defaults_file_invalid")
		}
	}
	return out.String(), nil
}

// Reject duplicate and case-variant fields before the encoding/json struct decoder.
func exactJSONFields(b []byte, t reflect.Type) error {
	d := json.NewDecoder(strings.NewReader(string(b)))
	var walk func(reflect.Type, int) error
	walk = func(t reflect.Type, depth int) error {
		if depth > 64 {
			return errors.New("json_depth")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') {
				return errors.New("json_object")
			}
			fields := map[string]reflect.Type{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag != "" && tag != "-" {
					fields[tag] = f.Type
				}
			}
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				ft, allowed := fields[key]
				if !ok || !allowed || seen[key] {
					return errors.New("json_field")
				}
				seen[key] = true
				if e = walk(ft, depth+1); e != nil {
					return e
				}
			}
			if len(seen) != len(fields) {
				return errors.New("json_missing_field")
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return errors.New("json_object_end")
			}
		case reflect.Slice:
			if token != json.Delim('[') {
				return errors.New("json_array")
			}
			n := 0
			for d.More() {
				n++
				if n > 10000 {
					return errors.New("json_array_limit")
				}
				if e := walk(t.Elem(), depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return errors.New("json_array_end")
			}
		default:
			if _, ok := token.(json.Delim); ok || token == nil {
				return errors.New("json_scalar")
			}
		}
		return nil
	}
	if err := walk(t, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("json_trailing")
	}
	return nil
}
