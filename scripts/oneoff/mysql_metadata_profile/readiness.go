package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

// Only this fixed reader uses the new argument. No SQL, scope, role or target
// can be supplied by its caller. DATABASE() is pinned to qs by runReadiness;
// the internal executor also serves independently guarded owned fixtures.
const readinessIdentitySQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ CONNECTION_ID(), @@server_uuid, DATABASE(), VERSION(), CURRENT_ROLE(), @@performance_schema, @@event_scheduler"
const readinessMigrationSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ version, dirty FROM schema_migrations LIMIT 2"
const readinessTablesSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TABLE_NAME, TABLE_TYPE, ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME LIMIT 1001"
const readinessViewsSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TABLE_NAME, CHECK_OPTION, IS_UPDATABLE FROM information_schema.VIEWS WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME LIMIT 1001"
const readinessRoutinesSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ ROUTINE_NAME, ROUTINE_TYPE, DATA_TYPE FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = DATABASE() ORDER BY ROUTINE_NAME, ROUTINE_TYPE LIMIT 1001"
const readinessTriggersSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TRIGGER_NAME, EVENT_OBJECT_TABLE, EVENT_MANIPULATION, ACTION_TIMING FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = DATABASE() ORDER BY TRIGGER_NAME LIMIT 1001"
const readinessEventsSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ EVENT_SCHEMA, EVENT_NAME, STATUS, EVENT_TYPE, LAST_EXECUTED FROM information_schema.EVENTS ORDER BY EVENT_SCHEMA, EVENT_NAME LIMIT 1001"
const readinessColumnsSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TABLE_NAME, COLUMN_NAME, DATA_TYPE, IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND ((TABLE_NAME = 'assessment' AND COLUMN_NAME = 'historical_lifecycle_evidence') OR (TABLE_NAME = 'evaluation_outcome' AND COLUMN_NAME = 'historical_committed_evidence')) ORDER BY TABLE_NAME LIMIT 1001"
const readinessChecksSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TABLE_NAME, CONSTRAINT_NAME, CONSTRAINT_TYPE, ENFORCED FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND ((TABLE_NAME = 'assessment' AND CONSTRAINT_NAME = 'chk_assessment_historical_lifecycle') OR (TABLE_NAME = 'evaluation_outcome' AND CONSTRAINT_NAME = 'chk_outcome_historical_committed')) ORDER BY TABLE_NAME LIMIT 1001"
const readinessThreadsSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ THREAD_ID, NAME, TYPE, PROCESSLIST_ID, PROCESSLIST_COMMAND FROM performance_schema.threads WHERE (TYPE = 'FOREGROUND' OR NAME LIKE 'thread/sql/event%') AND (PROCESSLIST_ID IS NULL OR PROCESSLIST_ID <> CONNECTION_ID()) ORDER BY THREAD_ID LIMIT 1001"
const readinessTransactionsSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ TRX_MYSQL_THREAD_ID, TRX_STATE, TRX_STARTED, TRX_ROWS_MODIFIED, TRX_ROWS_LOCKED, TRX_IS_READ_ONLY FROM information_schema.INNODB_TRX WHERE TRX_MYSQL_THREAD_ID <> CONNECTION_ID() ORDER BY TRX_MYSQL_THREAD_ID LIMIT 1001"
const readinessLocksSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ OBJECT_TYPE, OBJECT_SCHEMA, OBJECT_NAME, LOCK_TYPE, LOCK_DURATION, LOCK_STATUS, OWNER_THREAD_ID FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA = DATABASE() ORDER BY OWNER_THREAD_ID, OBJECT_TYPE, OBJECT_NAME LIMIT 1001"
const readinessInstrumentSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ NAME, ENABLED FROM performance_schema.setup_instruments WHERE NAME = 'wait/lock/metadata/sql/mdl' LIMIT 1001"
const readinessAssessmentSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ COUNT(*), COALESCE(SUM(historical_lifecycle_evidence IS NULL),0), COALESCE(SUM(historical_lifecycle_evidence IS NOT NULL),0), COALESCE(SUM(JSON_TYPE(historical_lifecycle_evidence) = 'NULL'),0), COALESCE(SUM(JSON_TYPE(historical_lifecycle_evidence) = 'OBJECT' AND JSON_LENGTH(historical_lifecycle_evidence) = 0),0), COALESCE(SUM(JSON_TYPE(historical_lifecycle_evidence) = 'OBJECT' AND JSON_LENGTH(historical_lifecycle_evidence) > 0),0), COALESCE(SUM(historical_lifecycle_evidence IS NOT NULL AND JSON_TYPE(historical_lifecycle_evidence) NOT IN ('NULL','OBJECT')),0) FROM assessment"
const readinessOutcomeSQL = "SELECT /*+ MAX_EXECUTION_TIME(5000) */ COUNT(*), COALESCE(SUM(historical_committed_evidence IS NULL),0), COALESCE(SUM(historical_committed_evidence IS NOT NULL),0), COALESCE(SUM(JSON_TYPE(historical_committed_evidence) = 'NULL'),0), COALESCE(SUM(JSON_TYPE(historical_committed_evidence) = 'OBJECT' AND JSON_LENGTH(historical_committed_evidence) = 0),0), COALESCE(SUM(JSON_TYPE(historical_committed_evidence) = 'OBJECT' AND JSON_LENGTH(historical_committed_evidence) > 0),0), COALESCE(SUM(historical_committed_evidence IS NOT NULL AND JSON_TYPE(historical_committed_evidence) NOT IN ('NULL','OBJECT')),0) FROM evaluation_outcome"

type readinessIdentity struct {
	identity
	PerformanceSchema uint64
	EventScheduler    string
}
type catalogSummary struct {
	Count  uint64 `json:"count"`
	Digest string `json:"digest"`
}
type scalarPartition struct {
	Total          uint64 `json:"total"`
	SQLNull        uint64 `json:"sql_null"`
	NonNull        uint64 `json:"non_null"`
	JSONNull       uint64 `json:"json_null"`
	EmptyObject    uint64 `json:"empty_object"`
	NonEmptyObject uint64 `json:"non_empty_object"`
	Other          uint64 `json:"other"`
}
type readinessMeasurements struct {
	GlobalGrantRowsVerified  bool                      `json:"global_metadata_grant_rows_verified"`
	MigrationVersion         uint64                    `json:"migration_version"`
	MigrationDirty           bool                      `json:"migration_dirty"`
	PerformanceSchemaEnabled bool                      `json:"performance_schema_enabled"`
	EventScheduler           string                    `json:"event_scheduler"`
	Catalogs                 map[string]catalogSummary `json:"catalogs"`
	EnabledEvents            uint64                    `json:"enabled_events"`
	Threads                  uint64                    `json:"threads"`
	EventThreads             uint64                    `json:"event_threads"`
	Transactions             uint64                    `json:"transactions"`
	ModifyingTransactions    uint64                    `json:"modifying_transactions"`
	MetadataLocks            uint64                    `json:"metadata_locks"`
	MDLInstrumentEnabled     bool                      `json:"mdl_instrument_enabled"`
	Assessment               scalarPartition           `json:"assessment_distribution"`
	Outcome                  scalarPartition           `json:"outcome_distribution"`
}
type readinessReceipt struct {
	FormatVersion      int                    `json:"format_version"`
	SourceSHA          string                 `json:"source_sha,omitempty"`
	RunID              string                 `json:"run_id,omitempty"`
	ExpectedTargetHash string                 `json:"expected_target_hash,omitempty"`
	SourceTargetHash   string                 `json:"source_target_hash,omitempty"`
	DiagnosticOnly     bool                   `json:"diagnostic_only"`
	ReadinessOnly      bool                   `json:"readiness_only"`
	FreezeVerified     bool                   `json:"freeze_verified"`
	Complete           bool                   `json:"complete"`
	ErrorCategory      string                 `json:"error_category"`
	FailureStage       string                 `json:"failure_stage"`
	QueryCount         uint64                 `json:"query_count"`
	ConnectionReleased *bool                  `json:"connection_released"`
	DatabaseReleased   *bool                  `json:"database_released"`
	PoolClosedVerified *bool                  `json:"pool_closed_verified"`
	Measurements       *readinessMeasurements `json:"measurements,omitempty"`
}

func baseReadinessReceipt(b binding) readinessReceipt {
	old := baseReceipt(b)
	return readinessReceipt{FormatVersion: 1, SourceSHA: old.SourceSHA, RunID: old.RunID, ExpectedTargetHash: old.ExpectedTargetHash, DiagnosticOnly: true, ReadinessOnly: true, ErrorCategory: "input_invalid", FailureStage: "input"}
}
func invalidateReadiness(r readinessReceipt, category, stage string) readinessReceipt {
	r.Complete, r.Measurements, r.ErrorCategory, r.FailureStage = false, nil, category, stage
	return r
}

// Only typed errors contribute a more specific public category. Driver text,
// account names and SQL never enter this closed diagnostic classification.
func readinessQueryError(err error) error {
	var server *mysql.MySQLError
	if errors.As(err, &server) && server != nil {
		switch server.Number {
		case 3024:
			return errors.New("query_server_timeout")
		case 1317:
			return errors.New("query_server_interrupted")
		case 1142, 1143, 1227:
			return errors.New("query_permission_denied")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("query_deadline_exceeded")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("query_canceled")
	}
	return errors.New("query_failed")
}

// Every returned row is consumed and every cursor release is checked. A
// 1001st row is a sentinel, never a silently omitted member of a directory.
// Operational rows remain RAM-only and are reduced to counts, not hashes of
// connection, transaction, role or account identifiers.
func readinessRows(ctx context.Context, c *sql.Conn, query string, width int) (result [][]sql.NullString, err error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, err := c.QueryContext(ctx, query)
	if err != nil {
		return nil, readinessQueryError(err)
	}
	defer func() {
		if rows.Close() != nil {
			result, err = nil, errors.New("metadata_release_failed")
		}
	}()
	columns, err := rows.Columns()
	if err != nil || len(columns) != width {
		return nil, errors.New("metadata_invalid")
	}
	bytes := 0
	for rows.Next() {
		if len(result) == 1000 {
			return nil, errors.New("metadata_truncated")
		}
		values := make([]sql.NullString, width)
		destinations := make([]any, width)
		for i := range values {
			destinations[i] = &values[i]
		}
		if rows.Scan(destinations...) != nil {
			return nil, errors.New("metadata_invalid")
		}
		for _, cell := range values {
			bytes += len(cell.String)
			if len(cell.String) > 65536 || bytes > 1024*1024 || !utf8.ValidString(cell.String) || strings.ContainsAny(cell.String, "\x00\r\n") {
				return nil, errors.New("metadata_invalid")
			}
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		return nil, readinessQueryError(err)
	}
	return result, nil
}
func summarizeCatalog(rows [][]sql.NullString) catalogSummary {
	raw, _ := json.Marshal(rows) // bounded sql.NullString cells cannot fail encoding
	digest := sha256.Sum256(raw)
	return catalogSummary{Count: uint64(len(rows)), Digest: hex.EncodeToString(digest[:])}
}
func readinessReadIdentity(ctx context.Context, c *sql.Conn) (readinessIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var v readinessIdentity
	err := c.QueryRowContext(ctx, readinessIdentitySQL).Scan(&v.ID, &v.UUID, &v.Database, &v.Version, &v.Roles, &v.PerformanceSchema, &v.EventScheduler)
	if err != nil || v.ID == 0 || !uuidRE.MatchString(v.UUID) || len(v.Database) == 0 || len(v.Database) > 255 || len(v.Version) == 0 || len(v.Version) > 128 || len(v.Roles) == 0 || len(v.Roles) > 65536 || v.PerformanceSchema > 1 {
		return readinessIdentity{}, errors.New("identity_read_failed")
	}
	switch v.EventScheduler {
	case "ON", "OFF", "DISABLED":
	default:
		return readinessIdentity{}, errors.New("identity_read_failed")
	}
	return v, nil
}
func readinessClean100(rows [][]sql.NullString) bool {
	if len(rows) != 1 || !rows[0][0].Valid || !rows[0][1].Valid {
		return false
	}
	version, err := strconv.ParseUint(rows[0][0].String, 10, 64)
	dirty, e := strconv.ParseBool(rows[0][1].String)
	return err == nil && e == nil && version == 100 && !dirty
}
func readinessTargetShape(columns, checks [][]sql.NullString) bool {
	if len(columns) != 2 || len(checks) != 2 {
		return false
	}
	for i, table := range []string{"assessment", "evaluation_outcome"} {
		column, check := "historical_lifecycle_evidence", "chk_assessment_historical_lifecycle"
		if i == 1 {
			column, check = "historical_committed_evidence", "chk_outcome_historical_committed"
		}
		for j, want := range []string{table, column, "json", "YES"} {
			if !columns[i][j].Valid || columns[i][j].String != want {
				return false
			}
		}
		for j, want := range []string{table, check, "CHECK", "YES"} {
			if !checks[i][j].Valid || checks[i][j].String != want {
				return false
			}
		}
	}
	return true
}
func readScalarPartition(rows [][]sql.NullString) (scalarPartition, bool) {
	var p scalarPartition
	if len(rows) != 1 {
		return p, false
	}
	destinations := []*uint64{&p.Total, &p.SQLNull, &p.NonNull, &p.JSONNull, &p.EmptyObject, &p.NonEmptyObject, &p.Other}
	for i, destination := range destinations {
		if !rows[0][i].Valid {
			return scalarPartition{}, false
		}
		value, err := strconv.ParseUint(rows[0][i].String, 10, 64)
		if err != nil || value > p.Total && i > 0 {
			return scalarPartition{}, false
		}
		*destination = value
	}
	// Subtraction avoids uint64 overflow even for malformed driver responses.
	if p.SQLNull > p.Total || p.NonNull != p.Total-p.SQLNull || p.JSONNull > p.NonNull || p.EmptyObject > p.NonNull-p.JSONNull || p.NonEmptyObject > p.NonNull-p.JSONNull-p.EmptyObject || p.Other != p.NonNull-p.JSONNull-p.EmptyObject-p.NonEmptyObject {
		return scalarPartition{}, false
	}
	return p, true
}
func probeReadiness(ctx context.Context, db *sql.DB, cfg connection, b binding) (r readinessReceipt) {
	r = baseReadinessReceipt(b)
	if !validBinding(b) {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, totalLimit)
	defer cancel()
	c, err := db.Conn(ctx)
	if err != nil {
		return invalidateReadiness(r, "connection_failed", "connect")
	}
	defer func() { r = releaseReadinessConnection(c, r) }()
	r.QueryCount++
	before, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return invalidateReadiness(r, "identity_read_failed", "identity_before")
	}
	if before.Database != cfg.Database {
		return invalidateReadiness(r, "target_identity_mismatch", "identity_before")
	}
	actual := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, before.UUID)
	if actual != b.Expected {
		return invalidateReadiness(r, "target_hash_mismatch", "identity_before")
	}
	r.SourceTargetHash = actual
	if before.PerformanceSchema != 1 {
		return invalidateReadiness(r, "storage_observation_unavailable", "identity_before")
	}
	read := func(query, stage string, width int) ([][]sql.NullString, bool) {
		r.QueryCount++
		rows, e := readinessRows(ctx, c, query, width)
		if e != nil {
			r = invalidateReadiness(r, e.Error(), stage)
			return nil, false
		}
		return rows, true
	}
	grantRows, ok := read(currentSQL, "global_grants", 1)
	if !ok {
		return r
	}
	currentRows := make([]string, 0, len(grantRows))
	for _, row := range grantRows {
		if !row[0].Valid {
			return invalidateReadiness(r, "metadata_invalid", "global_grants")
		}
		currentRows = append(currentRows, row[0].String)
	}
	full, known := unrestricted(currentRows)
	if !known || !full {
		return invalidateReadiness(r, "grants_unverified", "global_grants")
	}
	migrations, ok := read(readinessMigrationSQL, "migration_before", 2)
	if !ok {
		return r
	}
	if !readinessClean100(migrations) {
		return invalidateReadiness(r, "migration_pair_rejected", "migration_before")
	}
	m := &readinessMeasurements{GlobalGrantRowsVerified: true, MigrationVersion: 100, PerformanceSchemaEnabled: true, EventScheduler: strings.ToLower(before.EventScheduler), Catalogs: map[string]catalogSummary{}}
	catalogs := map[string][][]sql.NullString{}
	for _, step := range []struct {
		stage, query string
		width        int
	}{
		{"tables", readinessTablesSQL, 3}, {"views", readinessViewsSQL, 3}, {"routines", readinessRoutinesSQL, 3}, {"triggers", readinessTriggersSQL, 4}, {"events", readinessEventsSQL, 5}, {"columns", readinessColumnsSQL, 4}, {"checks", readinessChecksSQL, 4},
	} {
		rows, success := read(step.query, step.stage, step.width)
		if !success {
			return r
		}
		catalogs[step.stage] = rows
		m.Catalogs[step.stage] = summarizeCatalog(rows)
	}
	if !readinessTargetShape(catalogs["columns"], catalogs["checks"]) {
		return invalidateReadiness(r, "metadata_invalid", "columns")
	}
	var views uint64
	required := map[string]bool{"assessment": false, "evaluation_outcome": false, "schema_migrations": false}
	for _, row := range catalogs["tables"] {
		if !row[0].Valid || !row[1].Valid {
			return invalidateReadiness(r, "metadata_invalid", "tables")
		}
		switch row[1].String {
		case "VIEW":
			views++
		case "BASE TABLE":
			if _, needed := required[row[0].String]; needed {
				required[row[0].String] = row[2].Valid && row[2].String == "InnoDB"
			}
		default:
			return invalidateReadiness(r, "metadata_invalid", "tables")
		}
	}
	if views != m.Catalogs["views"].Count {
		return invalidateReadiness(r, "metadata_invalid", "views")
	}
	for _, present := range required {
		if !present {
			return invalidateReadiness(r, "metadata_invalid", "tables")
		}
	}
	for _, row := range catalogs["events"] {
		if !row[2].Valid {
			return invalidateReadiness(r, "metadata_invalid", "events")
		}
		switch row[2].String {
		case "ENABLED":
			m.EnabledEvents++
		case "DISABLED", "SLAVESIDE_DISABLED":
		default:
			return invalidateReadiness(r, "metadata_invalid", "events")
		}
	}
	threads, ok := read(readinessThreadsSQL, "threads", 5)
	if !ok {
		return r
	}
	m.Threads = uint64(len(threads))
	for _, row := range threads {
		if row[1].Valid && strings.HasPrefix(row[1].String, "thread/sql/event") {
			m.EventThreads++
		}
	}
	transactions, ok := read(readinessTransactionsSQL, "transactions", 6)
	if !ok {
		return r
	}
	m.Transactions = uint64(len(transactions))
	for _, row := range transactions {
		value, e := strconv.ParseUint(row[3].String, 10, 64)
		if e != nil || !row[3].Valid {
			return invalidateReadiness(r, "metadata_invalid", "transactions")
		}
		if value > 0 {
			m.ModifyingTransactions++
		}
	}
	locks, ok := read(readinessLocksSQL, "locks", 7)
	if !ok {
		return r
	}
	m.MetadataLocks = uint64(len(locks))
	instruments, ok := read(readinessInstrumentSQL, "instrument", 2)
	if !ok {
		return r
	}
	if len(instruments) != 1 || !instruments[0][0].Valid || instruments[0][0].String != "wait/lock/metadata/sql/mdl" || !instruments[0][1].Valid || instruments[0][1].String != "YES" {
		return invalidateReadiness(r, "storage_observation_unavailable", "instrument")
	}
	m.MDLInstrumentEnabled = true
	distribution, ok := read(readinessAssessmentSQL, "assessment_distribution", 7)
	if !ok {
		return r
	}
	m.Assessment, ok = readScalarPartition(distribution)
	if !ok {
		return invalidateReadiness(r, "metadata_invalid", "assessment_distribution")
	}
	distribution, ok = read(readinessOutcomeSQL, "outcome_distribution", 7)
	if !ok {
		return r
	}
	m.Outcome, ok = readScalarPartition(distribution)
	if !ok {
		return invalidateReadiness(r, "metadata_invalid", "outcome_distribution")
	}
	migrations, ok = read(readinessMigrationSQL, "migration_after", 2)
	if !ok {
		return r
	}
	if !readinessClean100(migrations) {
		return invalidateReadiness(r, "migration_pair_rejected", "migration_after")
	}
	r.QueryCount++
	after, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return invalidateReadiness(r, "identity_final_failed", "identity_after")
	}
	if before != after {
		return invalidateReadiness(r, "session_identity_changed", "identity_after")
	}
	r.ErrorCategory, r.FailureStage, r.Measurements = "none", "none", m
	// Complete is set only by the owned pool release, after this Conn's defer.
	return r
}
func releaseReadinessConnection(c *sql.Conn, r readinessReceipt) readinessReceipt {
	success := c.Close() == nil
	r.ConnectionReleased = &success
	if !success {
		return invalidateReadiness(r, "connection_release_failed", "connection_release")
	}
	return r
}
func releaseReadinessDatabase(db *sql.DB, r readinessReceipt) readinessReceipt {
	success := db.Close() == nil
	r.DatabaseReleased = &success
	stats := db.Stats()
	closed := stats.OpenConnections == 0 && stats.InUse == 0
	r.PoolClosedVerified = &closed
	if !success || !closed {
		return invalidateReadiness(r, "database_release_failed", "database_release")
	}
	if r.ErrorCategory == "none" && r.ConnectionReleased != nil && *r.ConnectionReleased && r.Measurements != nil {
		r.Complete = true
	}
	return r
}
func runReadiness(get func(string) string, out io.Writer) int {
	_ = mysql.SetLogger(&mysql.NopLogger{})
	b := binding{SHA: get("PROFILE_SOURCE_SHA"), RunID: get("PROFILE_RUN_ID"), Expected: get("PROFILE_EXPECTED_TARGET_HASH")}
	r := baseReadinessReceipt(b)
	if validBinding(b) {
		cfg, ok := envConnection(get)
		if !ok || cfg.Database != "qs" {
			r = invalidateReadiness(r, "connection_input_invalid", "input")
		} else {
			connector, err := mysql.NewConnector(mysqlConfig(cfg))
			if err != nil {
				r = invalidateReadiness(r, "connection_config_failed", "connect")
			} else {
				db := sql.OpenDB(connector)
				db.SetMaxOpenConns(1)
				db.SetMaxIdleConns(1)
				r = releaseReadinessDatabase(db, probeReadiness(context.Background(), db, cfg, b))
			}
		}
	}
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
