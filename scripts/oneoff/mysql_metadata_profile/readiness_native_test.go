//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This gate shares only the established native input guard. All fixture DDL,
// accounts and held transactions are owned by this test in a fresh loopback
// server. Production runReadiness still accepts only the literal qs database.
func TestMetadataReadinessNativeOwnedMySQL(t *testing.T) {
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" {
		t.Fatal("native_mysql_required_not_enabled")
	}
	cfg, ok := envConnection(os.Getenv)
	if !ok || cfg.Host != "127.0.0.1" || cfg.Port == 3306 || cfg.User != "root" || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(cfg.Database) {
		t.Fatal("owned_mysql_input_guard_failed")
	}
	connector, e := mysql.NewConnector(mysqlConfig(cfg))
	if e != nil {
		t.Fatal("owned_mysql_connector_failed")
	}
	admin := sql.OpenDB(connector)
	admin.SetMaxOpenConns(1)
	admin.SetMaxIdleConns(1)
	defer func() {
		if admin.Close() != nil {
			t.Error("owned_admin_release_failed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var uuid, version string
	var tables int
	if admin.QueryRowContext(ctx, "SELECT @@server_uuid, VERSION()").Scan(&uuid, &version) != nil || !uuidRE.MatchString(uuid) || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || version != "8.0.36" {
		t.Fatal("owned_mysql_identity_guard_failed")
	}
	if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()").Scan(&tables) != nil || tables != 0 {
		t.Fatal("owned_schema_not_empty")
	}
	eventDB := cfg.Database + "_events"
	// Cleanup addresses only the exact names created here. Neither current grant
	// setup nor migrations are applied to any pre-existing schema.
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, statement := range []string{"DROP DATABASE IF EXISTS `" + eventDB + "`", "DROP PROCEDURE IF EXISTS readiness_owned_proc", "DROP VIEW IF EXISTS readiness_owned_view", "DROP TABLE IF EXISTS assessment", "DROP TABLE IF EXISTS evaluation_outcome", "DROP TABLE IF EXISTS schema_migrations", "DROP USER IF EXISTS `readiness_global`@`%`", "DROP USER IF EXISTS `readiness_no_process`@`%`", "DROP USER IF EXISTS `readiness_writer`@`%`"} {
			if _, err := admin.ExecContext(cleanup, statement); err != nil {
				t.Error("owned_fixture_release_failed")
			}
		}
	}()
	for _, statement := range []string{
		"CREATE TABLE schema_migrations(version BIGINT UNSIGNED PRIMARY KEY,dirty BOOLEAN NOT NULL) ENGINE=InnoDB",
		"INSERT INTO schema_migrations VALUES(100,false)",
		"CREATE TABLE assessment(id BIGINT PRIMARY KEY,historical_lifecycle_evidence JSON NULL,CONSTRAINT chk_assessment_historical_lifecycle CHECK(historical_lifecycle_evidence IS NULL OR JSON_TYPE(historical_lifecycle_evidence)='OBJECT')) ENGINE=InnoDB",
		"CREATE TABLE evaluation_outcome(id BIGINT PRIMARY KEY,historical_committed_evidence JSON NULL,CONSTRAINT chk_outcome_historical_committed CHECK(historical_committed_evidence IS NULL OR JSON_TYPE(historical_committed_evidence)='OBJECT')) ENGINE=InnoDB",
		"INSERT INTO assessment VALUES(1,NULL),(2,JSON_OBJECT()),(3,JSON_OBJECT('fixture','native-readiness-private-body'))",
		"INSERT INTO evaluation_outcome VALUES(1,NULL)",
		"CREATE VIEW readiness_owned_view AS SELECT id FROM assessment",
		"CREATE PROCEDURE readiness_owned_proc() SELECT 1",
		"CREATE TRIGGER readiness_owned_trigger BEFORE INSERT ON assessment FOR EACH ROW SET NEW.id=NEW.id",
		"CREATE DATABASE `" + eventDB + "`",
		"CREATE EVENT `" + eventDB + "`.readiness_owned_event ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL 1 DAY DO SELECT 1",
		"CREATE USER `readiness_global`@`%` IDENTIFIED BY 'native-readiness-private-fixture'",
		"GRANT SELECT,SHOW VIEW,TRIGGER,EVENT,PROCESS ON *.* TO `readiness_global`@`%`",
		"CREATE USER `readiness_no_process`@`%` IDENTIFIED BY 'native-readiness-private-fixture'",
		"GRANT SELECT,SHOW VIEW,TRIGGER,EVENT ON *.* TO `readiness_no_process`@`%`",
		"CREATE USER `readiness_writer`@`%` IDENTIFIED BY 'native-readiness-private-fixture'",
		"GRANT SELECT,UPDATE ON `" + cfg.Database + "`.* TO `readiness_writer`@`%`",
	} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatal("owned_readiness_fixture_setup_failed")
		}
	}
	open := func(user string) *sql.DB {
		t.Helper()
		copy := cfg
		copy.User = user
		copy.Password = "native-readiness-private-fixture"
		con, err := mysql.NewConnector(mysqlConfig(copy))
		if err != nil {
			t.Fatal("owned_reader_connector_failed")
		}
		db := sql.OpenDB(con)
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		return db
	}
	b := binding{SHA: strings.Repeat("a", 40), RunID: "12345-1", Expected: hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)}
	reader := func(user string) readinessReceipt {
		t.Helper()
		db := open(user)
		r := releaseReadinessDatabase(db, probeReadiness(ctx, db, cfg, b))
		var remaining int
		if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE USER = ?", user).Scan(&remaining) != nil || remaining != 0 {
			t.Fatal("native_readiness_connection_not_released")
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal("native_readiness_receipt_invalid")
		}
		for _, private := range []string{uuid, user, cfg.Database, "native-readiness-private-fixture", "native-readiness-private-body", "readiness_owned_event", "readiness_owned_proc", "TRX_MYSQL_THREAD_ID", "CURRENT_ROLE"} {
			if bytes.Contains(raw, []byte(private)) {
				t.Fatal("native_readiness_private_value_leaked")
			}
		}
		if r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatal("native_readiness_release_unverified")
		}
		return r
	}
	t.Run("missing_process_real_privilege_error", func(t *testing.T) {
		r := reader("readiness_no_process")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_permission_denied" || r.FailureStage != "transactions" {
			t.Fatalf("native missing PROCESS not rejected: %s/%s", r.ErrorCategory, r.FailureStage)
		}
	})
	t.Run("server_timeout_real_3024_and_release", func(t *testing.T) {
		// Sole SELECT SLEEP can return a normal row when interrupted. This
		// fixture-only predicate forces a real server 3024, below the unchanged
		// five-second client deadline, without changing production SQL.
		db := open("readiness_global")
		c, err := db.Conn(ctx)
		if err != nil {
			_ = db.Close()
			t.Fatal("owned_timeout_connection_failed")
		}
		_, err = readinessRows(ctx, c, "SELECT /*+ MAX_EXECUTION_TIME(50) */ 1 FROM assessment WHERE SLEEP(1)", 1)
		r := baseReadinessReceipt(b)
		if err == nil {
			r = invalidateReadiness(r, "metadata_invalid", "tables")
		} else {
			r = invalidateReadiness(r, err.Error(), "tables")
		}
		r = releaseReadinessDatabase(db, releaseReadinessConnection(c, r))
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_server_timeout" || r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatalf("native server timeout classification or release failed: %s", r.ErrorCategory)
		}
		assertReadinessPrivate(t, r)
	})

	t.Run("all_schema_event_and_other_user_held_transaction", func(t *testing.T) {
		writer := open("readiness_writer")
		defer func() {
			if writer.Close() != nil {
				t.Error("owned_writer_release_failed")
			}
		}()
		txn, err := writer.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("owned_writer_begin_failed")
		}
		defer func() {
			if err := txn.Rollback(); err != nil && err != sql.ErrTxDone {
				t.Error("owned_writer_rollback_failed")
			}
		}()
		if _, err = txn.ExecContext(ctx, "UPDATE assessment SET historical_lifecycle_evidence=JSON_OBJECT('fixture','held-private-value') WHERE id=3"); err != nil {
			t.Fatal("owned_writer_mutation_failed")
		}
		r := reader("readiness_global")
		if !r.Complete || r.QueryCount != 18 || r.Measurements == nil {
			t.Fatalf("native readiness incomplete: %s/%s", r.ErrorCategory, r.FailureStage)
		}
		m := r.Measurements
		if m.Transactions < 1 || m.ModifyingTransactions < 1 || m.MetadataLocks < 1 || m.EnabledEvents < 1 || m.Catalogs["events"].Count < 1 || m.Catalogs["tables"].Count != 4 || m.Catalogs["views"].Count != 1 || m.Catalogs["routines"].Count != 1 || m.Catalogs["triggers"].Count != 1 {
			t.Fatal("native_readiness_hidden_metadata")
		}
		if m.Assessment != (scalarPartition{Total: 3, SQLNull: 1, NonNull: 2, EmptyObject: 1, NonEmptyObject: 1}) || m.Outcome != (scalarPartition{Total: 1, SQLNull: 1}) || r.FreezeVerified {
			t.Fatal("native_scalar_partition_or_scope_failed")
		}
	})
	t.Run("dirty_version_rejected", func(t *testing.T) {
		if _, err := admin.ExecContext(ctx, "UPDATE schema_migrations SET dirty=true WHERE version=100"); err != nil {
			t.Fatal("owned_version_mutation_failed")
		}
		r := reader("readiness_global")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "migration_pair_rejected" || r.FailureStage != "migration_before" {
			t.Fatal("native_dirty_version_accepted")
		}
		if _, err := admin.ExecContext(ctx, "UPDATE schema_migrations SET dirty=false WHERE version=100"); err != nil {
			t.Fatal("owned_version_restore_failed")
		}
	})
	t.Run("missing_target_shape_rejected", func(t *testing.T) {
		if _, err := admin.ExecContext(ctx, "ALTER TABLE evaluation_outcome DROP CHECK chk_outcome_historical_committed"); err != nil {
			t.Fatal("owned_shape_mutation_failed")
		}
		r := reader("readiness_global")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "metadata_invalid" {
			t.Fatal("native_missing_target_check_accepted")
		}
	})
}
