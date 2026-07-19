//go:build integration

package retrygovernance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestGovernanceSummaryReconcilesWithOrganizationCandidates(t *testing.T) {
	mysqlDB := openRetryGovernanceMySQL(t)
	mongoDB := openRetryGovernanceMongo(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	seedRetryGovernanceMySQL(t, mysqlDB, now)
	seedRetryGovernanceMongo(t, mongoDB, now)

	reader := NewReader(mysqlDB, mongoDB)
	summary, err := reader.ReadRetryGovernance(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	page, err := reader.ListRetryCandidates(t.Context(), 7, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Fatalf("unexpected next cursor %q", page.NextCursor)
	}

	type key struct{ kind, disposition string }
	counts := make(map[key]int64)
	for _, item := range page.Items {
		if item.ResourceID == "org-8-outbox" || item.ResourceID == "8001" || item.ResourceID == "8" || item.ResourceID == "hold-org-8" {
			t.Fatalf("organization 8 candidate leaked into organization 7: %#v", item)
		}
		counts[key{item.Kind, item.Disposition}]++
	}

	if got := counts[key{"evaluation", "automatic"}] + counts[key{"interpretation", "automatic"}]; got != summary.Automatic {
		t.Fatalf("automatic summary=%d candidates=%d", summary.Automatic, got)
	}
	if got := counts[key{"evaluation", "manual_required"}] + counts[key{"interpretation", "manual_required"}]; got != summary.ManualRequired {
		t.Fatalf("manual summary=%d candidates=%d", summary.ManualRequired, got)
	}
	if got := counts[key{"evaluation", "terminal"}] + counts[key{"interpretation", "terminal"}]; got != summary.Terminal {
		t.Fatalf("terminal summary=%d candidates=%d", summary.Terminal, got)
	}
	if got := counts[key{"outbox", "automatic"}]; got != summary.OutboxAutomatic {
		t.Fatalf("outbox automatic summary=%d candidates=%d", summary.OutboxAutomatic, got)
	}
	if got := counts[key{"outbox", "manual_required"}]; got != summary.OutboxManual {
		t.Fatalf("outbox manual summary=%d candidates=%d", summary.OutboxManual, got)
	}
	if got := counts[key{"transport_delivery", "manual_required"}]; got != summary.TransportDeadLetters {
		t.Fatalf("transport summary=%d candidates=%d", summary.TransportDeadLetters, got)
	}
	if got := counts[key{"retry_hold", "automatic"}]; got != summary.HeldAutomatic {
		t.Fatalf("held automatic summary=%d candidates=%d", summary.HeldAutomatic, got)
	}
	if got := counts[key{"retry_hold", "manual_required"}]; got != summary.HeldManualRequired {
		t.Fatalf("held manual summary=%d candidates=%d", summary.HeldManualRequired, got)
	}
	if summary.BlockedRetryEvents != 2 {
		t.Fatalf("blocked retry events = %d, want 2", summary.BlockedRetryEvents)
	}
}

func openRetryGovernanceMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required for retry governance integration tests")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_retry_reconcile_%d", time.Now().UnixNano())
	cfg.DBName = ""
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`") })
	cfg.DBName = databaseName
	cfg.ParseTime = true
	cfg.MultiStatements = true
	cfg.InterpolateParams = true
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(`
CREATE TABLE assessment (id bigint unsigned PRIMARY KEY, org_id bigint NOT NULL, deleted_at datetime(3) NULL);
CREATE TABLE runtime_checkpoint (
 id bigint unsigned AUTO_INCREMENT PRIMARY KEY, assessment_id bigint unsigned NOT NULL,
 attempt_no int NOT NULL, scope varchar(64) NOT NULL, status varchar(32) NOT NULL,
 retry_disposition varchar(32) NULL, next_attempt_at datetime(3) NULL,
 retry_event_id varchar(128) NULL, action_request_id varchar(128) NULL,
 updated_at datetime(3) NOT NULL, deleted_at datetime(3) NULL
);
CREATE TABLE evaluation_outcome (id bigint unsigned PRIMARY KEY, org_id bigint NOT NULL);
CREATE TABLE domain_event_outbox (
 event_id varchar(128) PRIMARY KEY, org_id bigint NULL, event_type varchar(128) NOT NULL,
 status varchar(32) NOT NULL, retry_disposition varchar(32) NULL, attempt_count int NOT NULL,
 next_attempt_at datetime(3) NULL, last_error_kind varchar(64) NULL, updated_at datetime(3) NOT NULL
);
CREATE TABLE event_delivery_dead_letter (
 id bigint unsigned AUTO_INCREMENT PRIMARY KEY, org_id bigint NULL,
 delivery_attempts int NOT NULL, last_error text NULL, retry_disposition varchar(32) NOT NULL,
 updated_at datetime(3) NOT NULL
);
CREATE TABLE retry_event_hold (
 event_id varchar(128) PRIMARY KEY, org_id bigint NULL, status varchar(32) NOT NULL,
 retry_disposition varchar(32) NOT NULL, replay_attempt_count int NOT NULL,
 next_attempt_at datetime(3) NULL, last_error text NULL, updated_at datetime(3) NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func openRetryGovernanceMongo(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI is required for retry governance integration tests")
	}
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database(fmt.Sprintf("qs_retry_reconcile_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return db
}

func seedRetryGovernanceMySQL(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO assessment (id,org_id) VALUES (1,7),(2,7),(3,7),(8,8);
INSERT INTO runtime_checkpoint (assessment_id,attempt_no,scope,status,retry_disposition,updated_at) VALUES
 (1,1,'evaluation_run','failed','manual_required',?),
 (1,2,'evaluation_run','failed','automatic',?),
 (2,1,'evaluation_run','failed','manual_required',?),
 (3,1,'evaluation_run','failed','terminal',?),
 (8,1,'evaluation_run','failed','automatic',?);
INSERT INTO evaluation_outcome (id,org_id) VALUES (101,7),(102,7),(103,7),(201,8);
INSERT INTO domain_event_outbox (event_id,org_id,event_type,status,retry_disposition,attempt_count,updated_at) VALUES
 ('mysql-auto',7,'evaluation.retry.requested','failed','automatic',2,?),
 ('mysql-manual',7,'evaluation.retry.requested','failed','manual_required',30,?),
 ('org-8-outbox',8,'evaluation.retry.requested','failed','manual_required',30,?);
INSERT INTO event_delivery_dead_letter (org_id,delivery_attempts,last_error,retry_disposition,updated_at) VALUES
 (7,8,'delivery failed','manual_required',?),(8,8,'delivery failed','manual_required',?);
INSERT INTO retry_event_hold (event_id,org_id,status,retry_disposition,replay_attempt_count,updated_at) VALUES
 ('hold-auto',7,'blocked','automatic',0,?),
 ('hold-manual',7,'failed','manual_required',30,?),
 ('hold-org-8',8,'blocked','automatic',0,?)`,
		now.Add(-time.Hour), now, now, now, now,
		now, now, now, now, now, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
}

func seedRetryGovernanceMongo(t *testing.T, db *mongo.Database, now time.Time) {
	t.Helper()
	if _, err := db.Collection("report_generations").InsertMany(t.Context(), []any{
		bson.M{"domain_id": int64(1001), "outcome_id": int64(101), "latest_run_id": int64(1001), "status": "failed", "deleted_at": nil},
		bson.M{"domain_id": int64(1002), "outcome_id": int64(102), "latest_run_id": int64(1002), "status": "failed", "deleted_at": nil},
		bson.M{"domain_id": int64(1003), "outcome_id": int64(103), "latest_run_id": int64(1003), "status": "failed", "deleted_at": nil},
		bson.M{"domain_id": int64(8001), "outcome_id": int64(201), "latest_run_id": int64(8001), "status": "failed", "deleted_at": nil},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("interpretation_runs").InsertMany(t.Context(), []any{
		bson.M{"domain_id": int64(1001), "status": "failed", "attempt": 1, "retry_disposition": "automatic", "updated_at": now},
		bson.M{"domain_id": int64(1002), "status": "failed", "attempt": 3, "retry_disposition": "manual_required", "updated_at": now},
		bson.M{"domain_id": int64(1003), "status": "succeeded", "attempt": 1, "retry_disposition": "automatic", "updated_at": now},
		bson.M{"domain_id": int64(8001), "status": "failed", "attempt": 1, "retry_disposition": "automatic", "updated_at": now},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("domain_event_outbox").InsertMany(t.Context(), []any{
		bson.M{"event_id": "mongo-auto", "org_id": int64(7), "event_type": "interpretation.retry.requested", "status": "failed", "retry_disposition": "automatic", "attempt_count": 2, "updated_at": now},
		bson.M{"event_id": "mongo-manual", "org_id": int64(7), "event_type": "interpretation.retry.requested", "status": "failed", "retry_disposition": "manual_required", "attempt_count": 30, "updated_at": now},
		bson.M{"event_id": "org-8-outbox", "org_id": int64(8), "event_type": "interpretation.retry.requested", "status": "failed", "retry_disposition": "manual_required", "attempt_count": 30, "updated_at": now},
	}); err != nil {
		t.Fatal(err)
	}
}
