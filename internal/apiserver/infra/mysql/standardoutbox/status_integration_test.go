//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestStandardMySQLStatusUsesCreationTimeAcrossSessionZones(t *testing.T) {
	dsn := os.Getenv("RM_QS_STATUS_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_status" {
		t.Fatal("disposable m4_qs_status MySQL at mysql:3306 required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // SET time_zone and the status read share this disposable session.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS qs_rm_replay_items")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS qs_rm_replay_requests")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS rm_outbox")
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../../../../../internal/pkg/migration/migrations/mysql/000084_standard_reliable_outbox.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(ddl), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, table := range []string{"qs_rm_replay_items", "qs_rm_replay_requests", "rm_outbox"} {
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		}
	}()
	if _, err := db.ExecContext(ctx, "SET time_zone = '+08:00'"); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC)
	for _, state := range []string{"pending", "retry_wait", "publishing", "quarantined", "published"} {
		_, err := db.ExecContext(ctx, `INSERT INTO rm_outbox
(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at,created_at)
VALUES ('qs-server',?,'events','created','v1','global','application/json','2026-09-23T10:00:00+08:00','{}',UNHEX(REPEAT('00',32)),?,'2026-09-23 02:00:00','2026-09-23 02:00:00')`, state, state)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader, err := NewStatusReader(db)
	if err != nil {
		t.Fatal(err)
	}
	observed := created.Add(2 * time.Hour).In(time.FixedZone("UTC+8", 8*3600))
	snapshot, err := reader.OutboxStatusSnapshot(ctx, observed)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Buckets) != 4 {
		t.Fatalf("unexpected buckets: %+v", snapshot)
	}
	for _, bucket := range snapshot.Buckets {
		if bucket.Count != 1 || bucket.OldestCreatedAt == nil || !bucket.OldestCreatedAt.Equal(created) || bucket.OldestAgeSeconds != 7200 {
			t.Fatalf("wrong state count or UTC age: %+v", bucket)
		}
	}
	_, err = db.ExecContext(ctx, "UPDATE rm_outbox SET state='unexpected' WHERE message_id='pending'")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.OutboxStatusSnapshot(ctx, observed); err == nil {
		t.Fatal("unknown unfinished state was hidden")
	}
}
