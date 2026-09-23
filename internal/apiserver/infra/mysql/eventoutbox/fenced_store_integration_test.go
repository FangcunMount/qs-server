package eventoutbox

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// This test uses a disposable MySQL database. Its schema keeps the legacy
// UTC+8 DATETIME columns and adds only the proposed delivery fence columns.
func TestFencedStoreMySQLLeaseAndTimezone(t *testing.T) {
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set QS_SERVER_TEST_MYSQL_DSN to a disposable MySQL database")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Net != "tcp" || !strings.HasPrefix(parsed.Addr, "127.0.0.1:") || parsed.DBName != "m4_fenced" {
		t.Fatal("fenced store test requires a localhost disposable m4_fenced database")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	// A connection pool may select either session. The FencedStore must use
	// the same absolute instant and legacy +08:00 DATETIME digits in both.
	if _, err := db.ExecContext(ctx, `SET time_zone = '+00:00'`); err != nil {
		t.Fatal(err)
	}
	assertSessionZone := func(want string) {
		t.Helper()
		var got string
		if err := db.QueryRowContext(ctx, `SELECT @@session.time_zone`).Scan(&got); err != nil || got != want {
			t.Fatalf("session time zone: got=%q want=%q err=%v", got, want, err)
		}
	}
	assertSessionZone("+00:00")
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS domain_event_outbox`); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE domain_event_outbox (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		event_id VARCHAR(64) NOT NULL UNIQUE,
		topic_name VARCHAR(128) NOT NULL,
		payload_json LONGTEXT NOT NULL,
		status VARCHAR(32) NOT NULL,
		attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
		retry_disposition VARCHAR(32) NULL,
		next_attempt_at DATETIME(3) NOT NULL,
		last_error TEXT NULL,
		last_error_kind VARCHAR(32) NULL,
		created_at DATETIME(3) NOT NULL,
		updated_at DATETIME(3) NOT NULL,
		published_at DATETIME(3) NULL,
		delivery_claim_token CHAR(64) NULL,
		delivery_lease_until DATETIME(6) NULL,
		delivery_claim_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
		delivery_attempt_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
		KEY idx_status_due (status,next_attempt_at)
	) ENGINE=InnoDB`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS domain_event_outbox`) })
	_, err = db.ExecContext(ctx, `INSERT INTO domain_event_outbox
		(event_id,topic_name,payload_json,status,attempt_count,next_attempt_at,created_at,updated_at)
		VALUES ('fenced-test','topic','{}','pending',2,`+fencedDBNow+` - INTERVAL 1 SECOND,`+fencedDBNow+`,`+fencedDBNow+`)`)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewFencedStore(db)
	if err != nil {
		t.Fatal(err)
	}
	const lease = time.Second
	claim := func() FencedClaim {
		t.Helper()
		rows, err := store.ClaimDue(ctx, 1, lease, time.Minute)
		if err != nil || len(rows) != 1 {
			t.Fatalf("claim: rows=%d err=%v", len(rows), err)
		}
		return rows[0]
	}
	a := claim()
	if a.FailureCount != 2 || a.DeliveryCount != 1 || a.LeaseUntil.Location().String() != "UTC+8" {
		t.Fatalf("first claim counters/timezone: %+v", a)
	}
	var dbClockText, leaseText string
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(`+fencedDBNow+`, '%Y-%m-%d %H:%i:%s.%f'),DATE_FORMAT(delivery_lease_until, '%Y-%m-%d %H:%i:%s.%f') FROM domain_event_outbox WHERE event_id='fenced-test'`).Scan(&dbClockText, &leaseText); err != nil {
		t.Fatal(err)
	}
	dbClock, err := time.ParseInLocation(fencedTimeLayout, dbClockText, fencedLocation)
	if err != nil {
		t.Fatal(err)
	}
	if offset := a.LeaseUntil.Sub(dbClock); offset <= 0 || offset > lease || leaseText != a.LeaseUntil.Format(fencedTimeLayout) {
		t.Fatalf("lease does not preserve +08:00 wall clock: offset=%s db=%s lease=%s", offset, dbClockText, leaseText)
	}
	time.Sleep(lease + 100*time.Millisecond)
	b := claim()
	if b.Token == a.Token || b.Version != a.Version+1 || b.FailureCount != 2 || b.DeliveryCount != 2 {
		t.Fatalf("reclaim did not preserve failure budget: a=%+v b=%+v", a, b)
	}
	if err := store.Confirm(ctx, a); !errors.Is(err, ErrStaleDeliveryClaim) {
		t.Fatalf("stale success writeback: %v", err)
	}
	if _, err := store.MarkFailedGoverned(ctx, a, "late failure"); !errors.Is(err, ErrStaleDeliveryClaim) {
		t.Fatalf("stale failure writeback: %v", err)
	}
	if err := store.Quarantine(ctx, a, "late encoding failure"); !errors.Is(err, ErrStaleDeliveryClaim) {
		t.Fatalf("stale quarantine writeback: %v", err)
	}
	if err := store.Confirm(ctx, b); err != nil {
		t.Fatalf("current owner confirm: %v", err)
	}
	var status string
	var failures int
	var deliveries uint64
	if err := db.QueryRowContext(ctx, `SELECT status,attempt_count,delivery_attempt_count FROM domain_event_outbox WHERE event_id='fenced-test'`).Scan(&status, &failures, &deliveries); err != nil {
		t.Fatal(err)
	}
	if status != "published" || failures != 2 || deliveries != 2 {
		t.Fatalf("final state: %s failures=%d deliveries=%d", status, failures, deliveries)
	}
	if _, err := db.ExecContext(ctx, `SET time_zone = '+08:00'`); err != nil {
		t.Fatal(err)
	}
	assertSessionZone("+08:00")
	_, err = db.ExecContext(ctx, `INSERT INTO domain_event_outbox
		(event_id,topic_name,payload_json,status,next_attempt_at,created_at,updated_at)
		VALUES ('fenced-test-shanghai','topic','{}','pending',`+fencedDBNow+` - INTERVAL 1 SECOND,`+fencedDBNow+`,`+fencedDBNow+`)`)
	if err != nil {
		t.Fatal(err)
	}
	shanghai := claim()
	if shanghai.EventID != "fenced-test-shanghai" || shanghai.FailureCount != 0 || shanghai.LeaseUntil.Location().String() != "UTC+8" {
		t.Fatalf("+08:00 session claim: %+v", shanghai)
	}
	if err := store.Confirm(ctx, shanghai); err != nil {
		t.Fatalf("+08:00 session confirm: %v", err)
	}
}
