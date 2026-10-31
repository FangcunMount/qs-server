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

func TestM5ActivityAdminOwnedNativeMySQL(t *testing.T) {
	cfg, ok := envConnection(os.Getenv)
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" || !ok || cfg.Host != "127.0.0.1" || cfg.Port == 3306 || cfg.User != "root" || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(cfg.Database) {
		t.Fatal("owned M5 activity input required")
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
	if _, e := db.ExecContext(ctx, "CREATE TABLE schema_migrations(version BIGINT UNSIGNED PRIMARY KEY,dirty BOOLEAN NOT NULL) ENGINE=InnoDB"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS schema_migrations") })
	if _, e := db.ExecContext(ctx, "INSERT INTO schema_migrations VALUES(110,FALSE)"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE TABLE m5_activity_owned(id INT PRIMARY KEY) ENGINE=InnoDB"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS m5_activity_owned") })
	if _, e := db.ExecContext(ctx, "INSERT INTO m5_activity_owned VALUES(1)"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.ExecContext(ctx, "CREATE USER m5_activity_owned_no_sys@'%' IDENTIFIED BY 'owned-activity-fixture'"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP USER IF EXISTS m5_activity_owned_no_sys@'%'")
	})
	if _, e := db.ExecContext(ctx, "GRANT SELECT ON `"+cfg.Database+"`.* TO m5_activity_owned_no_sys@'%'"); e != nil {
		t.Fatal(e)
	}
	shaRaw, e := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if e != nil {
		t.Fatal(e)
	}
	b := binding{SHA: strings.TrimSpace(string(shaRaw)), RunID: "1-1", Expected: hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)}
	read := func() m5ActivityReceipt {
		t.Helper()
		r := probeM5Activity(ctx, db, cfg, b)
		if !r.Complete || r.Measurements == nil || r.QueryCount != 14 || r.FreezeVerified || !r.ReadOnly || r.ObservationsAtomic || r.ConnectionReleased == nil || !*r.ConnectionReleased {
			t.Fatal("actual M5 activity unavailable", r)
		}
		return r
	}
	before := read()
	if before.Measurements.MySQLVersion != 110 || before.Measurements.MySQLDirty {
		t.Fatal("original110 not observed")
	}
	locker, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _, _ = locker.ExecContext(context.Background(), "UNLOCK TABLES"); _ = locker.Close() }()
	if _, e := locker.ExecContext(ctx, "LOCK TABLES m5_activity_owned READ"); e != nil {
		t.Fatal(e)
	}
	ddlCfg := cfg
	ddlDB := dbOpen(ddlCfg)
	done := make(chan error, 1)
	go func() {
		_, e := ddlDB.ExecContext(ctx, "ALTER TABLE m5_activity_owned ADD COLUMN fixture_only INT NULL")
		done <- e
	}()
	var busy m5ActivityReceipt
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		busy = read()
		if busy.Measurements.Counts["qs_metadata_locks_pending"] > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if busy.Measurements.Counts["qs_metadata_locks_pending"] == 0 || busy.Measurements.Counts["qs_metadata_locks_granted"] == 0 || busy.Measurements.Counts["qs_active_threads"] == 0 {
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
	limited.User = "m5_activity_owned_no_sys"
	limited.Password = "owned-activity-fixture"
	refused := probeM5Activity(ctx, dbOpen(limited), limited, b)
	if refused.Complete || refused.Measurements != nil || refused.FreezeVerified {
		t.Fatal("restricted visibility became zero")
	}
	if _, e := db.ExecContext(ctx, "UPDATE schema_migrations SET version=111,dirty=TRUE"); e != nil {
		t.Fatal(e)
	}
	dirty := read()
	if dirty.Measurements.MySQLVersion != 111 || !dirty.Measurements.MySQLDirty {
		t.Fatal("actual interrupted state hidden")
	}
	var count int
	var head uint64
	var actualDirty bool
	if db.QueryRowContext(ctx, "SELECT COUNT(*),MIN(version),MAX(dirty) FROM schema_migrations").Scan(&count, &head, &actualDirty) != nil || count != 1 || head != 111 || !actualDirty {
		t.Fatal("readonly activity modified ledger")
	}
	report := map[string]any{"source_sha": b.SHA, "fixture_only": true, "passed": true, "actual_pending_metadata_lock_and_DDL_observed": true, "nonzero_not_freeze": true, "restricted_visibility_refused": true, "actual110_and_dirty111_observed_without_writes": true, "production_or_system_acceptance": false}
	if path := os.Getenv("QS_M5_ACTIVITY_NATIVE_PROOF"); path != "" {
		raw, e := json.Marshal(report)
		if e != nil || os.WriteFile(path, append(raw, '\n'), 0600) != nil {
			t.Fatal("M5 activity proof write")
		}
	}
}
