//go:build integration

package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestRetryGovernanceMySQLMigrationsUpDown(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_SERVER_TEST_MONGO_REQUIRED") == "1" {
			t.Log("MYSQL_DSN is outside the Mongo-only integration gate")
			return
		}
		t.Skip("MYSQL_DSN is required for migration integration tests")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_retry_migration_%d", time.Now().UnixNano())
	cfg.DBName = ""
	cfg.MultiStatements = true
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`") }()
	cfg.DBName = databaseName
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE assessment (
 id bigint unsigned NOT NULL PRIMARY KEY, deleted_at datetime(3) NULL
);
CREATE TABLE runtime_checkpoint (
 id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
 assessment_id bigint unsigned NULL, attempt_no int NOT NULL, scope varchar(64) NOT NULL,
 status varchar(32) NOT NULL, retryable tinyint(1) NOT NULL DEFAULT 0, deleted_at datetime(3) NULL
);
CREATE TABLE domain_event_outbox (
 id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
 event_id varchar(64) NOT NULL UNIQUE, aggregate_id varchar(64) NOT NULL,
 payload_json longtext NOT NULL, status varchar(32) NOT NULL, attempt_count int NOT NULL DEFAULT 0,
 last_error text NULL, next_attempt_at datetime(3) NULL
);
INSERT INTO assessment (id) VALUES (1);
INSERT INTO runtime_checkpoint (assessment_id,attempt_no,scope,status,retryable) VALUES (1,1,'evaluation_run','failed',1);
INSERT INTO domain_event_outbox (event_id,aggregate_id,payload_json,status,attempt_count) VALUES ('event-1','1','{}','failed',30);`)
	if err != nil {
		t.Fatal(err)
	}
	execSQLMigration(t, db, "000049_add_retry_governance.up.sql")
	execSQLMigration(t, db, "000050_add_retry_event_hold.up.sql")
	assertMySQLColumn(t, db, databaseName, "runtime_checkpoint", "retry_disposition", true)
	assertMySQLColumn(t, db, databaseName, "retry_event_hold", "claim_token", true)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO retry_event_hold
(event_id,message_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at)
VALUES ('event-1','message-1','nsq','topic','channel','{}',1,'paused',NOW(3))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO retry_event_hold
(event_id,message_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at)
VALUES ('event-2','message-1','nsq','topic','channel','{}',1,'paused',NOW(3))`); err == nil {
		t.Fatal("retry hold unique delivery identity was not enforced")
	}
	execSQLMigration(t, db, "000050_add_retry_event_hold.down.sql")
	execSQLMigration(t, db, "000049_add_retry_governance.down.sql")
	assertMySQLColumn(t, db, databaseName, "runtime_checkpoint", "retry_disposition", false)
	assertMySQLColumn(t, db, databaseName, "retry_event_hold", "claim_token", false)
}

func TestRetryGovernanceMongoMigrationUpDown(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	if err := db.CreateCollection(t.Context(), "interpretation_runs"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(t.Context(), "domain_event_outbox"); err != nil {
		t.Fatal(err)
	}
	execMongoMigration(t, db, "000012_add_retry_governance.up.json")
	assertMongoIndex(t, db.Collection("interpretation_runs"), "idx_interpretation_run_retry_due", true)
	assertMongoIndex(t, db.Collection("domain_event_outbox"), "idx_outbox_org_retry_due", true)
	execMongoMigration(t, db, "000012_add_retry_governance.down.json")
	assertMongoIndex(t, db.Collection("interpretation_runs"), "idx_interpretation_run_retry_due", false)
}

func execSQLMigration(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("migrations", "mysql", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), string(payload)); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
}

func assertMySQLColumn(t *testing.T, db *sql.DB, databaseName, table, column string, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=? AND table_name=? AND column_name=?`, databaseName, table, column).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if (count == 1) != want {
		t.Fatalf("column %s.%s exists=%v, want %v", table, column, count == 1, want)
	}
}

func execMongoMigration(t *testing.T, db *mongo.Database, name string) {
	t.Helper()
	if err := runMongoMigration(t.Context(), db, name); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
}

func runMongoMigration(ctx context.Context, db *mongo.Database, name string) error {
	payload, err := os.ReadFile(filepath.Join("migrations", "mongodb", name))
	if err != nil {
		return err
	}
	var rawCommands []json.RawMessage
	if err := json.Unmarshal(payload, &rawCommands); err != nil {
		return err
	}
	commands := make([]bson.D, 0, len(rawCommands))
	for _, raw := range rawCommands {
		var command bson.D
		if err := bson.UnmarshalExtJSON(raw, true, &command); err != nil {
			return err
		}
		commands = append(commands, command)
	}
	for _, command := range commands {
		if err := db.RunCommand(ctx, command).Err(); err != nil {
			return err
		}
	}
	return nil
}

func assertMongoIndex(t *testing.T, collection *mongo.Collection, name string, want bool) {
	t.Helper()
	cursor, err := collection.Indexes().List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close(t.Context())
	found := false
	for cursor.Next(t.Context()) {
		var item struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&item); err != nil {
			t.Fatal(err)
		}
		found = found || item.Name == name
	}
	if found != want {
		t.Fatalf("index %s exists=%v, want %v", name, found, want)
	}
}
