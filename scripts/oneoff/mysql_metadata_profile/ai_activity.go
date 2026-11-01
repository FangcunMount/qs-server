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

	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	"github.com/go-sql-driver/mysql"
)

var aiActivityQueries = []struct{ name, query string }{
	{"ai_threads", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.threads WHERE PROCESSLIST_DB=DATABASE() AND PROCESSLIST_ID<>?"},
	{"ai_active_threads", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.threads WHERE PROCESSLIST_DB=DATABASE() AND PROCESSLIST_ID<>? AND PROCESSLIST_COMMAND<>'Sleep'"},
	{"ai_transactions", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.innodb_trx x JOIN performance_schema.threads t ON t.PROCESSLIST_ID=x.trx_mysql_thread_id WHERE t.PROCESSLIST_DB=DATABASE() AND t.PROCESSLIST_ID<>?"},
	{"ai_modifying_transactions", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.innodb_trx x JOIN performance_schema.threads t ON t.PROCESSLIST_ID=x.trx_mysql_thread_id WHERE t.PROCESSLIST_DB=DATABASE() AND t.PROCESSLIST_ID<>? AND x.trx_rows_modified>0"},
	{"ai_metadata_locks_granted", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA=DATABASE() AND LOCK_STATUS='GRANTED' AND (OWNER_THREAD_ID IS NULL OR OWNER_THREAD_ID<>?)"},
	{"ai_metadata_locks_pending", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA=DATABASE() AND LOCK_STATUS<>'GRANTED' AND (OWNER_THREAD_ID IS NULL OR OWNER_THREAD_ID<>?)"},
	{"global_metadata_locks_pending", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA IS NULL AND LOCK_STATUS<>'GRANTED' AND (OWNER_THREAD_ID IS NULL OR OWNER_THREAD_ID<>?)"},
	{"ai_enabled_events", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA=DATABASE() AND STATUS='ENABLED' AND ? > 0"},
	{"ai_triggers", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND ? > 0"},
	{"ai_routines", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=DATABASE() AND ? > 0"},
	{"ai_views", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.VIEWS WHERE TABLE_SCHEMA=DATABASE() AND ? > 0"},
	{"target_incoming_foreign_keys", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME='messaging_outbox' AND ? > 0"},
	{"target_external_foreign_keys", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME='messaging_outbox' AND TABLE_SCHEMA<>DATABASE() AND ? > 0"},
	{"target_view_dependencies", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.VIEW_TABLE_USAGE WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='messaging_outbox' AND ? > 0"},
}

const aiHead43 = "0043_native_transaction_limit"
const aiHead44 = "0044_empty_publisher"
const aiLedgerSQL = "SELECT /*+ MAX_EXECUTION_TIME(2000) */ version_num FROM alembic_version LIMIT 2"
const aiTargetPresenceSQL = "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='messaging_outbox'"

type aiActivityMeasurements struct {
	SchemaHead                   string            `json:"schema_head"`
	TargetPresent                bool              `json:"target_present"`
	TargetRows                   *uint64           `json:"target_rows,omitempty"`
	TargetDDLSHA256              *string           `json:"target_ddl_sha256,omitempty"`
	GlobalMetadataGrantsVerified bool              `json:"global_metadata_grants_verified"`
	Counts                       map[string]uint64 `json:"counts"`
	SampledAt                    uint64            `json:"sampled_at_epoch_microseconds"`
}
type aiActivityReceipt struct {
	FormatVersion       int                     `json:"format_version"`
	SourceSHA           string                  `json:"source_sha,omitempty"`
	RunID               string                  `json:"run_id,omitempty"`
	ExpectedTargetHash  string                  `json:"expected_target_hash,omitempty"`
	SourceTargetHash    string                  `json:"source_target_hash,omitempty"`
	MySQLIdentitySHA256 string                  `json:"mysql_identity_sha256,omitempty"`
	ReadOnly            bool                    `json:"read_only"`
	ActivityOnly        bool                    `json:"ai_activity_only"`
	FreezeVerified      bool                    `json:"freeze_verified"`
	ObservationsAtomic  bool                    `json:"observations_atomic"`
	Complete            bool                    `json:"complete"`
	QueryCount          uint64                  `json:"query_count"`
	ConnectionReleased  *bool                   `json:"connection_released"`
	DatabaseReleased    *bool                   `json:"database_released"`
	Measurements        *aiActivityMeasurements `json:"measurements,omitempty"`
	ErrorCategory       string                  `json:"error_category"`
	FailureStage        string                  `json:"failure_stage"`
	QueryMySQLNumber    *uint16                 `json:"query_mysql_error_number,omitempty"`
}

func baseAIActivity(b binding) aiActivityReceipt {
	r := aiActivityReceipt{FormatVersion: 1, ReadOnly: true, ActivityOnly: true, ErrorCategory: "input_invalid", FailureStage: "input"}
	if shaRE.MatchString(b.SHA) {
		r.SourceSHA = b.SHA
	}
	if runRE.MatchString(b.RunID) {
		r.RunID = b.RunID
	}
	if hashRE.MatchString(b.Expected) {
		r.ExpectedTargetHash = b.Expected
	}
	return r
}
func failAIActivity(r aiActivityReceipt, category, stage string, err error) aiActivityReceipt {
	r.Complete = false
	r.Measurements = nil
	r.ErrorCategory = category
	r.FailureStage = stage
	if err != nil {
		var q *readinessQueryFailure
		if errors.As(err, &q) || errors.As(readinessQueryError(err), &q) {
			r.ErrorCategory = q.category
			r.QueryMySQLNumber = q.number
		}
	}
	return r
}

// Independent AI administrator observation. Accepts only intact43 or intact44.
// It never changes the QS activity/readiness contracts or qualifies a writer freeze.
// Counts exclude this pinned session. They are sequential observations, not a
// writer freeze, no-first-start certificate or authorization to change schema.
func probeAIActivity(ctx context.Context, db *sql.DB, cfg connection, b binding) (r aiActivityReceipt) {
	r = baseAIActivity(b)
	if !validBinding(b) {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, totalLimit)
	defer cancel()
	c, err := db.Conn(ctx)
	if err != nil {
		return failAIActivity(r, "connection_failed", "connect", nil)
	}
	defer func() {
		err := c.Close()
		ok := err == nil
		r.ConnectionReleased = &ok
		if !ok {
			r = failAIActivity(r, "connection_release_failed", "connection_release", nil)
		}
	}()
	r.QueryCount++
	before, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return failAIActivity(r, "identity_read_failed", "identity_before", nil)
	}
	if before.Database != cfg.Database {
		return failAIActivity(r, "target_identity_mismatch", "identity_before", nil)
	}
	actual := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, before.UUID)
	if actual != b.Expected {
		return failAIActivity(r, "target_hash_mismatch", "identity_before", nil)
	}
	r.SourceTargetHash = actual
	if before.PerformanceSchema != 1 {
		return failAIActivity(r, "storage_observation_unavailable", "identity_before", nil)
	}
	read := func(query, stage string, width int, args ...any) ([][]sql.NullString, bool) {
		r.QueryCount++
		rows, e := readinessRows(ctx, c, query, width, args...)
		if e != nil {
			r = failAIActivity(r, "query_failed", stage, e)
			return nil, false
		}
		return rows, true
	}
	grants, ok := read(currentSQL, "global_grants", 1)
	if !ok {
		return r
	}
	rawGrants := []string{}
	for _, row := range grants {
		if !row[0].Valid {
			return failAIActivity(r, "metadata_invalid", "global_grants", nil)
		}
		rawGrants = append(rawGrants, row[0].String)
	}
	if allowed, known := unrestricted(rawGrants); !allowed || !known {
		return failAIActivity(r, "grants_unverified", "global_grants", nil)
	}
	ledger, ok := read(aiLedgerSQL, "ledger", 1)
	if !ok {
		return r
	}
	if len(ledger) != 1 || !ledger[0][0].Valid || (ledger[0][0].String != aiHead43 && ledger[0][0].String != aiHead44) {
		return failAIActivity(r, "metadata_invalid", "ledger", nil)
	}
	head := ledger[0][0].String
	present, ok := read(aiTargetPresenceSQL, "target_presence", 1)
	if !ok {
		return r
	}
	if len(present) != 1 || !present[0][0].Valid || (present[0][0].String != "0" && present[0][0].String != "1") {
		return failAIActivity(r, "metadata_invalid", "target_presence", nil)
	}
	targetPresent := present[0][0].String == "1"
	if (head == aiHead43) != targetPresent {
		return failAIActivity(r, "metadata_invalid", "target_presence", nil)
	}
	var targetRows *uint64
	var targetDDL *string
	if targetPresent {
		rows, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM messaging_outbox", "target_rows", 1)
		if !ok {
			return r
		}
		if len(rows) != 1 || !rows[0][0].Valid {
			return failAIActivity(r, "metadata_invalid", "target_rows", nil)
		}
		count, e := strconv.ParseUint(rows[0][0].String, 10, 64)
		if e != nil {
			return failAIActivity(r, "metadata_invalid", "target_rows", nil)
		}
		targetRows = &count
		ddl, ok := read("SHOW CREATE TABLE messaging_outbox", "target_ddl", 2)
		if !ok {
			return r
		}
		if len(ddl) != 1 || !ddl[0][0].Valid || ddl[0][0].String != "messaging_outbox" || !ddl[0][1].Valid || ddl[0][1].String == "" {
			return failAIActivity(r, "metadata_invalid", "target_ddl", nil)
		}
		sha := sha256.Sum256([]byte(ddl[0][1].String))
		text := hex.EncodeToString(sha[:])
		targetDDL = &text
	}
	instrument, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.setup_instruments WHERE NAME='wait/lock/metadata/sql/mdl' AND ENABLED='YES'", "instrument", 1)
	if !ok {
		return r
	}
	if len(instrument) != 1 || !instrument[0][0].Valid || instrument[0][0].String != "1" {
		return failAIActivity(r, "storage_observation_unavailable", "instrument", nil)
	}
	self, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=?", "thread_binding", 1, before.ID)
	if !ok {
		return r
	}
	if len(self) != 1 || !self[0][0].Valid {
		return failAIActivity(r, "storage_observation_unavailable", "thread_binding", nil)
	}
	threadID, e := strconv.ParseUint(self[0][0].String, 10, 64)
	if e != nil || threadID == 0 {
		return failAIActivity(r, "metadata_invalid", "thread_binding", nil)
	}
	m := aiActivityMeasurements{SchemaHead: head, TargetPresent: targetPresent, TargetRows: targetRows, TargetDDLSHA256: targetDDL, GlobalMetadataGrantsVerified: true, Counts: map[string]uint64{}}
	for _, q := range aiActivityQueries {
		identity := before.ID
		if strings.HasPrefix(q.name, "ai_metadata_locks_") || q.name == "global_metadata_locks_pending" {
			identity = threadID
		}
		rows, ok := read(q.query, q.name, 1, identity)
		if !ok {
			return r
		}
		if len(rows) != 1 || !rows[0][0].Valid {
			return failAIActivity(r, "metadata_invalid", q.name, nil)
		}
		n, e := strconv.ParseUint(rows[0][0].String, 10, 64)
		if e != nil {
			return failAIActivity(r, "metadata_invalid", q.name, nil)
		}
		m.Counts[q.name] = n
	}
	r.QueryCount++
	if err := c.QueryRowContext(ctx, "SELECT CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6))*1000000 AS UNSIGNED)").Scan(&m.SampledAt); err != nil {
		return failAIActivity(r, "query_failed", "sample_time", err)
	}
	if m.SampledAt == 0 {
		return failAIActivity(r, "metadata_invalid", "sample_time", nil)
	}
	r.QueryCount++
	after, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return failAIActivity(r, "identity_final_failed", "identity_after", nil)
	}
	if before != after {
		return failAIActivity(r, "session_identity_changed", "identity_after", nil)
	}
	raw, _ := json.Marshal([]string{before.UUID, before.Database})
	sum := sha256.Sum256(raw)
	r.MySQLIdentitySHA256 = hex.EncodeToString(sum[:])
	r.Measurements = &m
	r.Complete = true
	r.ErrorCategory = "none"
	r.FailureStage = "none"
	return r
}
func runAIActivity(get func(string) string, out io.Writer) int {
	_ = mysql.SetLogger(&mysql.NopLogger{})
	b := binding{SHA: get("PROFILE_SOURCE_SHA"), RunID: get("PROFILE_RUN_ID"), Expected: get("PROFILE_EXPECTED_TARGET_HASH")}
	r := baseAIActivity(b)
	if validBinding(b) {
		cfg, ok := envConnection(get)
		if !ok || cfg.Database != "ai" {
			r = failAIActivity(r, "connection_input_invalid", "input", nil)
		} else {
			connector, err := mysqltls.NewConnector(mysqlConfig(cfg))
			if err != nil {
				r = failAIActivity(r, "connection_config_failed", "connect", nil)
			} else {
				db := sql.OpenDB(connector)
				db.SetMaxOpenConns(1)
				db.SetMaxIdleConns(1)
				r = probeAIActivity(context.Background(), db, cfg, b)
				err = db.Close()
				released := err == nil
				r.DatabaseReleased = &released
				if err != nil {
					r = failAIActivity(r, "database_release_failed", "database_release", nil)
				}
			}
		}
	}
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
