//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
)

// All tables are created only inside an explicitly named disposable database.
// The production migration and runtime attachment are separate M4 tasks.
func TestStandardMySQLReplayLedgerCrashAndRollback(t *testing.T) {
	dsn := os.Getenv("RM_QS_REPLAY_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "127.0.0.1:3306" || parsed.DBName != "m4_qs_replay" {
		t.Fatal("disposable local m4_qs_replay MySQL required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.PingContext(ctx))
	for _, ddl := range []string{
		"DROP TRIGGER IF EXISTS block_replay_item",
		"DROP TABLE IF EXISTS qs_rm_replay_items",
		"DROP TABLE IF EXISTS qs_rm_replay_requests",
		"DROP TABLE IF EXISTS rm_outbox",
		sdkmysql.Schema,
		"ALTER TABLE rm_outbox ADD COLUMN manual_replay_request_id VARBINARY(64) NULL",
		`CREATE TABLE qs_rm_replay_requests (
 org_id BIGINT NOT NULL, request_id VARBINARY(64) NOT NULL,
 store_name VARBINARY(64) NOT NULL, reason VARCHAR(1024) NOT NULL,
 input_hash BINARY(32) NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
 PRIMARY KEY (org_id,request_id)) ENGINE=InnoDB`,
		`CREATE TABLE qs_rm_replay_items (
 org_id BIGINT NOT NULL, request_id VARBINARY(64) NOT NULL, ordinal SMALLINT UNSIGNED NOT NULL,
 event_id VARBINARY(128) NOT NULL, expected_failure_count BIGINT UNSIGNED NOT NULL,
 authorized TINYINT(1) NOT NULL, reason VARCHAR(64) NOT NULL DEFAULT '',
 PRIMARY KEY (org_id,request_id,ordinal),
 CONSTRAINT fk_qs_rm_replay_request FOREIGN KEY (org_id,request_id)
 REFERENCES qs_rm_replay_requests(org_id,request_id)) ENGINE=InnoDB`,
	} {
		_, err := db.ExecContext(ctx, ddl)
		must(err)
	}
	defer func() {
		for _, ddl := range []string{
			"DROP TRIGGER IF EXISTS block_replay_item",
			"DROP TABLE IF EXISTS qs_rm_replay_items",
			"DROP TABLE IF EXISTS qs_rm_replay_requests",
			"DROP TABLE IF EXISTS rm_outbox",
		} {
			_, _ = db.ExecContext(context.Background(), ddl)
		}
	}()
	appendQuarantined := func(id, scope, code string, failures uint64) {
		t.Helper()
		m, err := message.New(message.Input{
			Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle",
			EventType: "evaluation.requested", SchemaVersion: "v1", Scope: scope,
			ContentType: "application/json", OccurredAt: "2026-09-23T10:00:00+08:00",
			Payload: []byte(`{"event_id":"` + id + `"}`),
		})
		must(err)
		tx, err := db.BeginTx(ctx, nil)
		must(err)
		defer tx.Rollback()
		a, err := sdkmysql.Bind(tx)
		must(err)
		must(a.Append(ctx, m, time.Now().Add(-time.Minute)))
		_, err = tx.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code=?,failure_count=? WHERE message_id=?`, code, failures, id)
		must(err)
		must(tx.Commit())
	}
	ledger, err := NewReplayLedger(db, "assessment-mysql-outbox")
	must(err)
	appendQuarantined("event-a", "org:7", "publish_unknown", 30)
	first := request.ReplayRequest{
		OrgID: 7, RequestID: "request-1", Store: "assessment-mysql-outbox", Reason: "reviewed by operator",
		Targets: []request.ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 30}},
	}
	results, err := ledger.Authorize(ctx, first)
	must(err)
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("first authorization: %+v", results)
	}
	var state, manualID string
	var failures, version, attempts uint64
	var payload []byte
	must(db.QueryRowContext(ctx, `SELECT state,manual_replay_request_id,failure_count,version,attempt_count,payload FROM rm_outbox WHERE message_id='event-a'`).Scan(&state, &manualID, &failures, &version, &attempts, &payload))
	if state != "retry_wait" || manualID != first.RequestID || failures != 30 || version != 1 || attempts != 0 || string(payload) != `{"event_id":"event-a"}` {
		t.Fatalf("authorization changed wrong fields: %s %s %d %d %d %s", state, manualID, failures, version, attempts, payload)
	}
	// A later delivery and authorization must not erase the first request's
	// durable result when the HTTP reply or ActionAudit completion was lost.
	_, err = db.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code='publish_unknown',failure_count=31 WHERE message_id='event-a'`)
	must(err)
	second := first
	second.RequestID = "request-2"
	second.Targets = []request.ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 31}}
	results, err = ledger.Authorize(ctx, second)
	must(err)
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("second authorization: %+v", results)
	}
	replayed, err := ledger.Authorize(ctx, first)
	must(err)
	if len(replayed) != 1 || !replayed[0].Authorized {
		t.Fatalf("lost first audit result: %+v", replayed)
	}
	must(db.QueryRowContext(ctx, `SELECT manual_replay_request_id,failure_count,version FROM rm_outbox WHERE message_id='event-a'`).Scan(&manualID, &failures, &version))
	if manualID != second.RequestID || failures != 31 || version != 2 {
		t.Fatalf("old replay mutated newer authorization: %s %d %d", manualID, failures, version)
	}
	changed := first
	changed.Reason = "different reason"
	if _, err := ledger.Authorize(ctx, changed); !errors.Is(err, ErrReplayInputConflict) {
		t.Fatalf("same request ID accepted changed input: %v", err)
	}
	appendQuarantined("other-org", "org:8", "publish_unknown", 30)
	appendQuarantined("terminal", "org:7", "publish_rejected", 1)
	denied := request.ReplayRequest{
		OrgID: 7, RequestID: "request-denied", Store: "assessment-mysql-outbox", Reason: "reviewed",
		Targets: []request.ReplayTarget{
			{EventID: "other-org", ExpectedFailureCount: 30},
			{EventID: "terminal", ExpectedFailureCount: 1},
			{EventID: "missing", ExpectedFailureCount: 30},
		},
	}
	results, err = ledger.Authorize(ctx, denied)
	must(err)
	if len(results) != 3 || results[0].Reason != "organization_mismatch" || results[1].Reason != "not_manual_required" || results[2].Reason != "not_found" {
		t.Fatalf("denial categories changed: %+v", results)
	}
	// A failure after one row update must roll back both the authorization
	// and the request header; retrying the same request can then execute once.
	appendQuarantined("rollback-a", "org:7", "publish_unknown", 30)
	appendQuarantined("rollback-b", "org:7", "publish_unknown", 30)
	_, err = db.ExecContext(ctx, `CREATE TRIGGER block_replay_item BEFORE INSERT ON qs_rm_replay_items
 FOR EACH ROW BEGIN IF NEW.event_id='rollback-b' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='controlled failure'; END IF; END`)
	must(err)
	batch := request.ReplayRequest{
		OrgID: 7, RequestID: "request-rollback", Store: "assessment-mysql-outbox", Reason: "reviewed",
		Targets: []request.ReplayTarget{
			{EventID: "rollback-a", ExpectedFailureCount: 30},
			{EventID: "rollback-b", ExpectedFailureCount: 30},
		},
	}
	if _, err := ledger.Authorize(ctx, batch); err == nil {
		t.Fatal("controlled mid-batch failure unexpectedly committed")
	}
	for _, id := range []string{"rollback-a", "rollback-b"} {
		must(db.QueryRowContext(ctx, `SELECT state FROM rm_outbox WHERE message_id=?`, id).Scan(&state))
		if state != "quarantined" {
			t.Fatalf("%s escaped rolled-back batch: %s", id, state)
		}
	}
	var n int
	must(db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qs_rm_replay_requests WHERE request_id='request-rollback'`).Scan(&n))
	if n != 0 {
		t.Fatal("failed batch left a committed request header")
	}
	_, err = db.ExecContext(ctx, "DROP TRIGGER block_replay_item")
	must(err)
	results, err = ledger.Authorize(ctx, batch)
	must(err)
	if len(results) != 2 || !results[0].Authorized || !results[1].Authorized {
		t.Fatalf("retry after rollback: %+v", results)
	}
	appendQuarantined("concurrent", "org:7", "publish_unknown", 30)
	parallel := request.ReplayRequest{
		OrgID: 7, RequestID: "request-concurrent", Store: "assessment-mysql-outbox", Reason: "reviewed",
		Targets: []request.ReplayTarget{{EventID: "concurrent", ExpectedFailureCount: 30}},
	}
	type outcome struct {
		results []request.ReplayResult
		err     error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			items, err := ledger.Authorize(ctx, parallel)
			outcomes <- outcome{items, err}
		}()
	}
	for range 2 {
		got := <-outcomes
		must(got.err)
		if len(got.results) != 1 || !got.results[0].Authorized {
			t.Fatalf("parallel replay lost original result: %+v", got.results)
		}
	}
	must(db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='concurrent'`).Scan(&version))
	if version != 1 {
		t.Fatalf("parallel replay authorized %d times", version)
	}
}
