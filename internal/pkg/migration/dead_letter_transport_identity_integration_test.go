//go:build integration

package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestDeadLetterTransportIdentityMigrationPreservesLegacyRowsAndRefusesLossyDown(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required for migration integration tests")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_dead_letter_identity_%d", time.Now().UnixNano())
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
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE event_delivery_dead_letter (
 id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
 message_id varchar(128) NOT NULL,
 event_id varchar(64) NULL,
 org_id bigint NULL,
 provider varchar(32) NOT NULL,
 topic_name varchar(128) NOT NULL,
 channel_name varchar(128) NOT NULL,
 delivery_attempts int unsigned NOT NULL,
 payload_json longtext NOT NULL,
 last_error text NULL,
 retry_disposition varchar(32) NOT NULL,
 replay_request_id varchar(64) NULL,
 replayed_at datetime(3) NULL,
 failed_at datetime(3) NOT NULL,
 created_at datetime(3) NOT NULL,
 updated_at datetime(3) NOT NULL,
 UNIQUE KEY uk_delivery_dead_letter_identity (provider,topic_name,channel_name,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO event_delivery_dead_letter
 (message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,failed_at,created_at,updated_at)
 VALUES ('event-7','event-7',7,'nsq','topic','channel',8,'{}','archived_mock',NOW(3),NOW(3),NOW(3))`); err != nil {
		t.Fatal(err)
	}
	execSQLMigration(t, db, "000086_dead_letter_transport_identity.up.sql")
	var legacyPhysicalID, legacyState string
	if err := db.QueryRowContext(t.Context(), `SELECT transport_message_id,retry_disposition FROM event_delivery_dead_letter WHERE message_id='event-7'`).Scan(&legacyPhysicalID, &legacyState); err != nil {
		t.Fatal(err)
	}
	if legacyPhysicalID != "" || legacyState != "archived_mock" {
		t.Fatalf("legacy mock record changed: physical=%q state=%q", legacyPhysicalID, legacyState)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO event_delivery_dead_letter
 (message_id,transport_message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,failed_at,created_at,updated_at)
 VALUES ('event-7','replayed-physical-id','event-7',7,'nsq','topic','channel',8,'{}','manual_required',NOW(3),NOW(3),NOW(3))`); err != nil {
		t.Fatal(err)
	}
	downSQL, err := os.ReadFile(filepath.Join("migrations", "mysql", "000086_dead_letter_transport_identity.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), string(downSQL)); err == nil {
		t.Fatal("rollback accepted two physical failures with one logical UUID")
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter WHERE message_id='event-7'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("failed rollback changed failure records: count=%d err=%v", count, err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM event_delivery_dead_letter WHERE transport_message_id='replayed-physical-id'`); err != nil {
		t.Fatal(err)
	}
	execSQLMigration(t, db, "000086_dead_letter_transport_identity.down.sql")
	assertMySQLColumn(t, db, databaseName, "event_delivery_dead_letter", "transport_message_id", false)
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_delivery_dead_letter WHERE message_id='event-7' AND retry_disposition='archived_mock'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("reversible legacy row changed: count=%d err=%v", count, err)
	}
}
