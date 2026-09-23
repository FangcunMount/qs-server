//go:build integration && reliable_messaging_m4_integration

package migration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
)

func TestStandardReliableOutboxMySQLMigrationPreservesWrittenMessages(t *testing.T) {
	dsn := os.Getenv("RM_QS_M4_MIGRATION_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_migration" || !parsed.MultiStatements {
		t.Fatal("disposable m4_qs_migration MySQL with multiStatements=true required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	config := ensureConfigDefaults(&Config{Enabled: true, Database: parsed.DBName})
	instance, err := NewMySQLDriver(db).CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Force(83); err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(84); err != nil {
		t.Fatalf("apply standard schema: %v", err)
	}
	for _, table := range []string{"rm_outbox", "qs_rm_replay_requests", "qs_rm_replay_items"} {
		var exists int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name=?`, table).Scan(&exists); err != nil || exists != 1 {
			t.Fatalf("missing standard table %s: exists=%d err=%v", table, exists, err)
		}
	}
	// Before the first write, the migration itself can be rolled back.
	if err := instance.Migrate(83); err != nil {
		t.Fatalf("empty-schema rollback failed: %v", err)
	}
	if err := instance.Migrate(84); err != nil {
		t.Fatalf("reapply standard schema after empty rollback: %v", err)
	}
	// The released v0.1.0 appender must remain usable before the candidate SDK
	// Store with failure_count is deployed.
	m, err := message.New(message.Input{
		Producer: "qs-server", ID: "m4-new-message", Destination: "qs.evaluation.lifecycle",
		EventType: "evaluation.requested", SchemaVersion: "v1", Scope: "org:7",
		ContentType: "application/json", OccurredAt: "2026-09-23T10:00:00+08:00", Payload: []byte(`{"event_id":"m4-new-message"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	appender, err := sdkmysql.Bind(tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := appender.Append(t.Context(), m, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	store, err := sdkmysql.New(db)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := store.ClaimDue(t.Context(), 1, time.Minute)
	if err != nil || len(claims) != 1 || claims[0].Message.Input().ID != "m4-new-message" {
		t.Fatalf("released SDK could not claim new schema: claims=%+v err=%v", claims, err)
	}
	if err := store.Confirm(t.Context(), claims[0]); err != nil {
		t.Fatalf("released SDK could not confirm new schema: %v", err)
	}
	if err := instance.Migrate(83); err == nil {
		t.Fatal("rollback discarded or accepted a committed standard message")
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM rm_outbox WHERE message_id='m4-new-message'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed downgrade changed new message: count=%d err=%v", count, err)
	}
}

func TestStandardReliableOutboxMySQLColdStartReachesLatestSchema(t *testing.T) {
	dsn := os.Getenv("RM_QS_M4_MIGRATION_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_migration" || !parsed.MultiStatements {
		t.Fatal("disposable m4_qs_migration MySQL with multiStatements=true required")
	}
	parsed.DBName = ""
	server, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	const databaseName = "m4_qs_migration_cold"
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE "+databaseName); err != nil {
		t.Fatal(err)
	}
	defer server.ExecContext(context.Background(), "DROP DATABASE "+databaseName)
	parsed.DBName = databaseName
	db, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	version, changed, err := NewMigrator(db, &Config{Enabled: true, Database: databaseName}).Run()
	if err != nil || !changed || version != latestEmbeddedMySQLMigrationVersion(t) {
		t.Fatalf("cold-start MySQL standard schema: version=%d changed=%t err=%v", version, changed, err)
	}
	for _, column := range []string{"failure_count", "manual_replay_request_id", "manual_replay_version", "updated_at"} {
		var found int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='rm_outbox' AND column_name=?`, column).Scan(&found); err != nil || found != 1 {
			t.Fatalf("cold-start standard Outbox missing %s: found=%d err=%v", column, found, err)
		}
	}
}

func TestStandardReliableOutboxMongoMigrationRetainsCollectionsOnDowngrade(t *testing.T) {
	if os.Getenv("RM_QS_M4_MIGRATION_MYSQL_DSN") == "" {
		t.Fatal("run this isolated migration batch with both database gates")
	}
	client, db := mongodbtest.ReplicaSetDatabase(t)
	config := ensureConfigDefaults(&Config{Enabled: true, Database: db.Name()})
	instance, err := NewMongoDriver(client).CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Force(35); err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(36); err != nil {
		t.Fatalf("apply Mongo standard indexes: %v", err)
	}
	for collection, names := range map[string][]string{
		"rm_outbox":             {"ix_rm_outbox_due", "ix_rm_outbox_lease", "ix_rm_outbox_message_id", "ix_rm_outbox_scope_governance"},
		"qs_rm_replay_requests": {"ix_qs_rm_replay_requests_org_time"},
	} {
		cursor, err := db.Collection(collection).Indexes().List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var indexes []struct {
			Name string `bson:"name"`
		}
		if err := cursor.All(t.Context(), &indexes); err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, item := range indexes {
			found[item.Name] = true
		}
		for _, name := range names {
			if !found[name] {
				t.Fatalf("%s missing %s: %+v", collection, name, indexes)
			}
		}
	}
	_, err = db.Collection("rm_outbox").InsertOne(t.Context(), bson.M{"_id": "new-message", "message_id": "new-message"})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(35); err != nil {
		t.Fatalf("non-destructive Mongo downgrade failed: %v", err)
	}
	if err := instance.Migrate(36); err != nil {
		t.Fatalf("Mongo reapply with retained data failed: %v", err)
	}
	count, err := db.Collection("rm_outbox").CountDocuments(t.Context(), bson.M{"_id": "new-message"})
	if err != nil || count != 1 {
		t.Fatalf("Mongo downgrade removed new message: count=%d err=%v", count, err)
	}
}

func TestStandardReliableOutboxMongoColdStartReachesLatestIndexes(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	version, changed, err := NewMongoMigrator(client, &Config{Enabled: true, Database: db.Name()}).Run()
	if err != nil || !changed || version != latestEmbeddedMongoMigrationVersion(t) {
		t.Fatalf("cold-start Mongo standard indexes: version=%d changed=%t err=%v", version, changed, err)
	}
	for collection, indexName := range map[string]string{
		"rm_outbox": "ix_rm_outbox_due", "qs_rm_replay_requests": "ix_qs_rm_replay_requests_org_time",
	} {
		cursor, err := db.Collection(collection).Indexes().List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var rows []struct {
			Name string `bson:"name"`
		}
		if err := cursor.All(t.Context(), &rows); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			found = found || row.Name == indexName
		}
		if !found {
			t.Fatalf("cold-start %s missing %s: %+v", collection, indexName, rows)
		}
	}
}
