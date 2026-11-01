//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestAIActivityAdminOwnedNativeMySQL(t *testing.T) {
	cfg, ok := envConnection(os.Getenv)
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" || !ok || cfg.Host != "127.0.0.1" || cfg.Port == 3306 || cfg.User != "root" || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(cfg.Database) {
		t.Fatal("owned AI activity input required")
	}
	dbOpen := func(c connection) *sql.DB {
		t.Helper()
		connector, e := mysql.NewConnector(mysqlConfig(c))
		if e != nil {
			t.Fatal(e)
		}
		db := sql.OpenDB(connector)
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(2)
		t.Cleanup(func() {
			if db.Close() != nil {
				t.Error("owned activity connection release")
			}
		})
		return db
	}
	db := dbOpen(cfg)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	var uuid, version string
	var tables int
	if db.QueryRowContext(ctx, "SELECT @@server_uuid,VERSION()").Scan(&uuid, &version) != nil || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || version != "8.0.36" {
		t.Fatal("owned exact MySQL identity required")
	}
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables) != nil || tables != 0 {
		t.Fatal("fresh owned schema required")
	}
	if _, e := db.ExecContext(ctx, "CREATE TABLE alembic_version(version_num VARCHAR(32) PRIMARY KEY) ENGINE=InnoDB"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS alembic_version") })
	if _, e := db.ExecContext(ctx, "INSERT INTO alembic_version VALUES('0043_native_transaction_limit')"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE TABLE ai_activity_owned(id INT PRIMARY KEY) ENGINE=InnoDB"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS ai_activity_owned") })
	if _, e := db.ExecContext(ctx, "INSERT INTO ai_activity_owned VALUES(1)"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE USER ai_activity_owned_no_sys@'%' IDENTIFIED BY 'owned-activity-fixture'"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP USER IF EXISTS ai_activity_owned_no_sys@'%'")
	})
	if _, e := db.ExecContext(ctx, "GRANT SELECT ON `"+cfg.Database+"`.* TO ai_activity_owned_no_sys@'%'"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE TABLE messaging_outbox(id INT PRIMARY KEY,body VARBINARY(16)) ENGINE=InnoDB"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS messaging_outbox") })
	if _, e := db.ExecContext(ctx, "INSERT INTO messaging_outbox VALUES(1,0x00ff03)"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE VIEW ai_activity_owned_view AS SELECT id FROM messaging_outbox"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP VIEW IF EXISTS ai_activity_owned_view") })
	shaRaw, e := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if e != nil {
		t.Fatal(e)
	}
	b := binding{SHA: strings.TrimSpace(string(shaRaw)), RunID: "1-1", Expected: hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)}
	read := func() aiActivityReceipt {
		t.Helper()
		r := probeAIActivity(ctx, db, cfg, b)
		if !r.Complete || r.Measurements == nil || r.QueryCount != 24 && r.QueryCount != 22 || r.FreezeVerified || !r.ReadOnly || r.ObservationsAtomic || r.ConnectionReleased == nil || !*r.ConnectionReleased {
			t.Fatal("actual AI activity unavailable", r)
		}
		return r
	}
	before := read()
	if before.Measurements.SchemaHead != aiHead43 || before.Measurements.TargetRows == nil || *before.Measurements.TargetRows != 1 || before.Measurements.Counts["target_view_dependencies"] != 1 {
		t.Fatal("original110 not observed")
	}
	locker, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _, _ = locker.ExecContext(context.Background(), "UNLOCK TABLES"); _ = locker.Close() }()
	if _, e := locker.ExecContext(ctx, "LOCK TABLES ai_activity_owned READ"); e != nil {
		t.Fatal(e)
	}
	ddlCfg := cfg
	ddlDB := dbOpen(ddlCfg)
	done := make(chan error, 1)
	go func() {
		_, e := ddlDB.ExecContext(ctx, "ALTER TABLE ai_activity_owned ADD COLUMN fixture_only INT NULL")
		done <- e
	}()
	var busy aiActivityReceipt
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		busy = read()
		if busy.Measurements.Counts["ai_metadata_locks_pending"] > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if busy.Measurements.Counts["ai_metadata_locks_pending"] == 0 || busy.Measurements.Counts["ai_metadata_locks_granted"] == 0 || busy.Measurements.Counts["ai_active_threads"] == 0 {
		t.Fatal("actual held metadata/DDL positive control hidden", busy)
	}
	if _, e := locker.ExecContext(ctx, "UNLOCK TABLES"); e != nil {
		t.Fatal(e)
	}
	if e := locker.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("owned DDL not released")
	}
	limited := cfg
	limited.User = "ai_activity_owned_no_sys"
	limited.Password = "owned-activity-fixture"
	refused := probeAIActivity(ctx, dbOpen(limited), limited, b)
	if refused.Complete || refused.Measurements != nil || refused.FreezeVerified {
		t.Fatal("restricted visibility became zero")
	}
	var body string
	if db.QueryRowContext(ctx, "SELECT HEX(body) FROM messaging_outbox WHERE id=1").Scan(&body) != nil || body != "00FF03" {
		t.Fatal("observer changed original binary fact")
	}
	var original string
	if db.QueryRowContext(ctx, "SELECT version_num FROM alembic_version").Scan(&original) != nil || original != aiHead43 {
		t.Fatal("observer changed ledger")
	}
	// Explicit fixture-only transition; this is not a production retirement action.
	if _, e := db.ExecContext(ctx, "DROP VIEW ai_activity_owned_view"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "DROP TABLE messaging_outbox"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "UPDATE alembic_version SET version_num='0044_empty_publisher'"); e != nil {
		t.Fatal(e)
	}
	current := read()
	if current.Measurements.SchemaHead != aiHead44 || current.Measurements.TargetPresent || current.Measurements.TargetRows != nil || current.Measurements.TargetDDLSHA256 != nil {
		t.Fatal("44 absence became fake empty")
	}
	report := map[string]any{"source_sha": b.SHA, "fixture_only": true, "passed": true, "actual_pending_metadata_lock_and_DDL_observed": true, "nonzero_not_freeze": true, "restricted_visibility_refused": true, "actual43_and44_observed_without_observer_writes": true, "original_binary_fact_and_ledger_preserved": true, "view_dependency_detected": true, "production_or_system_acceptance": false}
	if path := os.Getenv("QS_AI_ACTIVITY_NATIVE_PROOF"); path != "" {
		raw, e := json.Marshal(report)
		if e != nil || os.WriteFile(path, append(raw, '\n'), 0600) != nil {
			t.Fatal("AI activity proof write")
		}
	}
}
