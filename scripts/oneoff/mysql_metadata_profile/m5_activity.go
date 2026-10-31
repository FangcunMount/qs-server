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

var m5ActivityQueries = []struct{ name, query string }{
	{"qs_threads", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.threads WHERE PROCESSLIST_DB=DATABASE() AND PROCESSLIST_ID<>?"},
	{"qs_active_threads", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.threads WHERE PROCESSLIST_DB=DATABASE() AND PROCESSLIST_ID<>? AND PROCESSLIST_COMMAND<>'Sleep'"},
	{"qs_transactions", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.innodb_trx x JOIN performance_schema.threads t ON t.PROCESSLIST_ID=x.trx_mysql_thread_id WHERE t.PROCESSLIST_DB=DATABASE() AND t.PROCESSLIST_ID<>?"},
	{"qs_modifying_transactions", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.innodb_trx x JOIN performance_schema.threads t ON t.PROCESSLIST_ID=x.trx_mysql_thread_id WHERE t.PROCESSLIST_DB=DATABASE() AND t.PROCESSLIST_ID<>? AND x.trx_rows_modified>0"},
	{"qs_metadata_locks_granted", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA=DATABASE() AND LOCK_STATUS='GRANTED' AND (OWNER_THREAD_ID IS NULL OR OWNER_THREAD_ID<>?)"},
	{"qs_metadata_locks_pending", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_SCHEMA=DATABASE() AND LOCK_STATUS<>'GRANTED' AND (OWNER_THREAD_ID IS NULL OR OWNER_THREAD_ID<>?)"},
	{"qs_enabled_events", "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA=DATABASE() AND STATUS='ENABLED' AND ? > 0"},
}

type m5ActivityMeasurements struct {
	MySQLVersion uint64            `json:"mysql_version"`
	MySQLDirty   bool              `json:"mysql_dirty"`
	Counts       map[string]uint64 `json:"counts"`
	SampledAt    uint64            `json:"sampled_at_epoch_microseconds"`
}
type m5ActivityReceipt struct {
	FormatVersion       int                     `json:"format_version"`
	SourceSHA           string                  `json:"source_sha,omitempty"`
	RunID               string                  `json:"run_id,omitempty"`
	ExpectedTargetHash  string                  `json:"expected_target_hash,omitempty"`
	SourceTargetHash    string                  `json:"source_target_hash,omitempty"`
	MySQLIdentitySHA256 string                  `json:"mysql_identity_sha256,omitempty"`
	ReadOnly            bool                    `json:"read_only"`
	ActivityOnly        bool                    `json:"m5_activity_only"`
	FreezeVerified      bool                    `json:"freeze_verified"`
	ObservationsAtomic  bool                    `json:"observations_atomic"`
	Complete            bool                    `json:"complete"`
	QueryCount          uint64                  `json:"query_count"`
	ConnectionReleased  *bool                   `json:"connection_released"`
	DatabaseReleased    *bool                   `json:"database_released"`
	Measurements        *m5ActivityMeasurements `json:"measurements,omitempty"`
	ErrorCategory       string                  `json:"error_category"`
	FailureStage        string                  `json:"failure_stage"`
	QueryMySQLNumber    *uint16                 `json:"query_mysql_error_number,omitempty"`
}

func baseM5Activity(b binding) m5ActivityReceipt {
	r := m5ActivityReceipt{FormatVersion: 1, ReadOnly: true, ActivityOnly: true, ErrorCategory: "input_invalid", FailureStage: "input"}
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
func failM5Activity(r m5ActivityReceipt, category, stage string, err error) m5ActivityReceipt {
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

// This separate activity observation accepts the actual ledger, including dirty
// interruption states. It never changes or relaxes the old clean100 readiness.
// Counts exclude this pinned session. They are sequential observations, not a
// writer freeze, no-first-start certificate or authorization to change schema.
func probeM5Activity(ctx context.Context, db *sql.DB, cfg connection, b binding) (r m5ActivityReceipt) {
	r = baseM5Activity(b)
	if !validBinding(b) {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, totalLimit)
	defer cancel()
	c, err := db.Conn(ctx)
	if err != nil {
		return failM5Activity(r, "connection_failed", "connect", nil)
	}
	defer func() {
		err := c.Close()
		ok := err == nil
		r.ConnectionReleased = &ok
		if !ok {
			r = failM5Activity(r, "connection_release_failed", "connection_release", nil)
		}
	}()
	r.QueryCount++
	before, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return failM5Activity(r, "identity_read_failed", "identity_before", nil)
	}
	if before.Database != cfg.Database {
		return failM5Activity(r, "target_identity_mismatch", "identity_before", nil)
	}
	actual := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, before.UUID)
	if actual != b.Expected {
		return failM5Activity(r, "target_hash_mismatch", "identity_before", nil)
	}
	r.SourceTargetHash = actual
	if before.PerformanceSchema != 1 {
		return failM5Activity(r, "storage_observation_unavailable", "identity_before", nil)
	}
	read := func(query, stage string, width int, args ...any) ([][]sql.NullString, bool) {
		r.QueryCount++
		rows, e := readinessRows(ctx, c, query, width, args...)
		if e != nil {
			r = failM5Activity(r, "query_failed", stage, e)
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
			return failM5Activity(r, "metadata_invalid", "global_grants", nil)
		}
		rawGrants = append(rawGrants, row[0].String)
	}
	if allowed, known := unrestricted(rawGrants); !allowed || !known {
		return failM5Activity(r, "grants_unverified", "global_grants", nil)
	}
	ledger, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ version,dirty FROM schema_migrations LIMIT 2", "ledger", 2)
	if !ok {
		return r
	}
	if len(ledger) != 1 || !ledger[0][0].Valid || !ledger[0][1].Valid {
		return failM5Activity(r, "metadata_invalid", "ledger", nil)
	}
	version, e := strconv.ParseUint(ledger[0][0].String, 10, 64)
	dirty, dirtyErr := strconv.ParseBool(ledger[0][1].String)
	if e != nil || dirtyErr != nil {
		return failM5Activity(r, "metadata_invalid", "ledger", nil)
	}
	instrument, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.setup_instruments WHERE NAME='wait/lock/metadata/sql/mdl' AND ENABLED='YES'", "instrument", 1)
	if !ok {
		return r
	}
	if len(instrument) != 1 || !instrument[0][0].Valid || instrument[0][0].String != "1" {
		return failM5Activity(r, "storage_observation_unavailable", "instrument", nil)
	}
	self, ok := read("SELECT /*+ MAX_EXECUTION_TIME(2000) */ THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=?", "thread_binding", 1, before.ID)
	if !ok {
		return r
	}
	if len(self) != 1 || !self[0][0].Valid {
		return failM5Activity(r, "storage_observation_unavailable", "thread_binding", nil)
	}
	threadID, e := strconv.ParseUint(self[0][0].String, 10, 64)
	if e != nil || threadID == 0 {
		return failM5Activity(r, "metadata_invalid", "thread_binding", nil)
	}
	m := m5ActivityMeasurements{MySQLVersion: version, MySQLDirty: dirty, Counts: map[string]uint64{}}
	for _, q := range m5ActivityQueries {
		identity := before.ID
		if strings.HasPrefix(q.name, "qs_metadata_locks_") {
			identity = threadID
		}
		rows, ok := read(q.query, q.name, 1, identity)
		if !ok {
			return r
		}
		if len(rows) != 1 || !rows[0][0].Valid {
			return failM5Activity(r, "metadata_invalid", q.name, nil)
		}
		n, e := strconv.ParseUint(rows[0][0].String, 10, 64)
		if e != nil {
			return failM5Activity(r, "metadata_invalid", q.name, nil)
		}
		m.Counts[q.name] = n
	}
	r.QueryCount++
	if err := c.QueryRowContext(ctx, "SELECT CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6))*1000000 AS UNSIGNED)").Scan(&m.SampledAt); err != nil {
		return failM5Activity(r, "query_failed", "sample_time", err)
	}
	if m.SampledAt == 0 {
		return failM5Activity(r, "metadata_invalid", "sample_time", nil)
	}
	r.QueryCount++
	after, err := readinessReadIdentity(ctx, c)
	if err != nil {
		return failM5Activity(r, "identity_final_failed", "identity_after", nil)
	}
	if before != after {
		return failM5Activity(r, "session_identity_changed", "identity_after", nil)
	}
	raw, _ := json.Marshal([]string{"mysql_server_selected_v1", before.UUID, before.Database})
	sum := sha256.Sum256(raw)
	r.MySQLIdentitySHA256 = hex.EncodeToString(sum[:])
	r.Measurements = &m
	r.Complete = true
	r.ErrorCategory = "none"
	r.FailureStage = "none"
	return r
}
func runM5Activity(get func(string) string, out io.Writer) int {
	_ = mysql.SetLogger(&mysql.NopLogger{})
	b := binding{SHA: get("PROFILE_SOURCE_SHA"), RunID: get("PROFILE_RUN_ID"), Expected: get("PROFILE_EXPECTED_TARGET_HASH")}
	r := baseM5Activity(b)
	if validBinding(b) {
		cfg, ok := envConnection(get)
		if !ok || cfg.Database != "qs" {
			r = failM5Activity(r, "connection_input_invalid", "input", nil)
		} else {
			connector, err := mysqltls.NewConnector(mysqlConfig(cfg))
			if err != nil {
				r = failM5Activity(r, "connection_config_failed", "connect", nil)
			} else {
				db := sql.OpenDB(connector)
				db.SetMaxOpenConns(1)
				db.SetMaxIdleConns(1)
				r = probeM5Activity(context.Background(), db, cfg, b)
				err = db.Close()
				released := err == nil
				r.DatabaseReleased = &released
				if err != nil {
					r = failM5Activity(r, "database_release_failed", "database_release", nil)
				}
			}
		}
	}
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
