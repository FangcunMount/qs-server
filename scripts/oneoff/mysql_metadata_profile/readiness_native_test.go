//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
		"CREATE TABLE assessment(id BIGINT UNSIGNED PRIMARY KEY,historical_lifecycle_evidence JSON NULL,CONSTRAINT chk_assessment_historical_lifecycle CHECK(historical_lifecycle_evidence IS NULL OR JSON_TYPE(historical_lifecycle_evidence)='OBJECT')) ENGINE=InnoDB",
		"CREATE TABLE evaluation_outcome(id BIGINT UNSIGNED PRIMARY KEY,historical_committed_evidence JSON NULL,CONSTRAINT chk_outcome_historical_committed CHECK(historical_committed_evidence IS NULL OR JSON_TYPE(historical_committed_evidence)='OBJECT')) ENGINE=InnoDB",
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
		r.QueryCount = 1
		if err == nil {
			r = invalidateReadiness(r, "metadata_invalid", "tables")
		} else {
			r = invalidateReadinessQuery(r, err, "tables")
		}
		r = releaseReadinessDatabase(db, releaseReadinessConnection(c, r))
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_server_timeout" || r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatalf("native server timeout classification or release failed: %s", r.ErrorCategory)
		}
		assertReadinessPrivate(t, r)
	})

	t.Run("unknown_server_real_1064_and_release", func(t *testing.T) {
		db := open("readiness_global")
		c, err := db.Conn(ctx)
		if err != nil {
			_ = db.Close()
			t.Fatal("owned_syntax_connection_failed")
		}
		_, err = readinessRows(ctx, c, "SELECT FROM", 1)
		r := baseReadinessReceipt(b)
		r.QueryCount = 1
		if err == nil {
			r = invalidateReadiness(r, "metadata_invalid", "tables")
		} else {
			r = invalidateReadinessQuery(r, err, "tables")
		}
		r = releaseReadinessDatabase(db, releaseReadinessConnection(c, r))
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_mysql_error" || r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1064 || r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatal("native_unknown_server_number_or_release_failed")
		}
		assertReadinessPrivate(t, r)
	})
	t.Run("read_timeout_real_invalid_connection_without_number", func(t *testing.T) {
		// Only this fixture's client deadline is shortened. Production remains
		// five seconds; a live context cannot turn this packet timeout into a
		// claimed context/server timeout after the driver has generalized it.
		copy := cfg
		copy.User, copy.Password = "readiness_global", "native-readiness-private-fixture"
		options := mysqlConfig(copy)
		options.ReadTimeout = 250 * time.Millisecond
		connector, err := mysql.NewConnector(options)
		if err != nil {
			t.Fatal("owned_short_read_connector_failed")
		}
		db := sql.OpenDB(connector)
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		c, err := db.Conn(ctx)
		if err != nil {
			_ = db.Close()
			t.Fatal("owned_short_read_connection_failed")
		}
		_, err = readinessRows(ctx, c, "SELECT SLEEP(1)", 1)
		r := baseReadinessReceipt(b)
		r.QueryCount = 1
		if err == nil {
			r = invalidateReadiness(r, "metadata_invalid", "tables")
		} else {
			r = invalidateReadinessQuery(r, err, "tables")
		}
		r = releaseReadinessDatabase(db, releaseReadinessConnection(c, r))
		if ctx.Err() != nil || r.Complete || r.Measurements != nil || r.ErrorCategory != "query_invalid_connection" || r.QueryMySQLNumber != nil || r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatal("native_read_timeout_was_misclassified_or_unreleased")
		}
		assertReadinessPrivate(t, r)
		// Server-side SLEEP may finish after the client disconnects. Observe only
		// this fixture's exact account and require its native session to settle.
		deadline := time.Now().Add(3 * time.Second)
		for {
			var remaining int
			if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE USER = ?", copy.User).Scan(&remaining) != nil {
				t.Fatal("owned_short_read_session_check_failed")
			}
			if remaining == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("owned_short_read_server_session_unsettled")
			}
			time.Sleep(25 * time.Millisecond)
		}
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
		if !r.Complete || r.QueryCount != 23 || r.ControlStatementCount != 3 || r.TransactionReleased == nil || !*r.TransactionReleased || r.FormatVersion != 2 || r.Measurements == nil {
			t.Fatalf("native readiness incomplete: %s/%s", r.ErrorCategory, r.FailureStage)
		}
		m := r.Measurements
		if !m.PrimaryKeyShapeVerified || !m.ConsistentSnapshot || !m.AssessmentEOF || !m.OutcomeEOF || m.AssessmentPages != 2 || m.OutcomePages != 2 {
			t.Fatal("native_pagination_not_complete")
		}
		if m.Transactions < 1 || m.ModifyingTransactions < 1 || m.MetadataLocks < 1 || m.EnabledEvents < 1 || m.Catalogs["events"].Count < 1 || m.Catalogs["tables"].Count != 4 || m.Catalogs["views"].Count != 1 || m.Catalogs["routines"].Count != 1 || m.Catalogs["triggers"].Count != 1 {
			t.Fatal("native_readiness_hidden_metadata")
		}
		if m.Assessment != (scalarPartition{Total: 3, SQLNull: 1, NonNull: 2, EmptyObject: 1, NonEmptyObject: 1}) || m.Outcome != (scalarPartition{Total: 1, SQLNull: 1}) || r.FreezeVerified {
			t.Fatal("native_scalar_partition_or_scope_failed")
		}
	})
	t.Run("same_rr_snapshot_survives_concurrent_insert_update_delete", func(t *testing.T) {
		// Only this owned fixture is changed. Both MAX values are established by
		// one consistent read; later page reads must not see committed new facts.
		for _, statement := range []string{
			"INSERT INTO assessment VALUES(0,NULL),(18446744073709551615,JSON_OBJECT())",
			"INSERT INTO evaluation_outcome VALUES(0,JSON_OBJECT()),(18446744073709551615,NULL)",
		} {
			if _, err := admin.ExecContext(ctx, statement); err != nil {
				t.Fatal("owned_snapshot_setup_failed")
			}
		}
		defer func() {
			for _, statement := range []string{
				"DELETE FROM assessment WHERE id IN (0,4,18446744073709551615) OR id BETWEEN 1000 AND 2000",
				"INSERT INTO assessment VALUES(1,NULL)",
				"UPDATE assessment SET historical_lifecycle_evidence=JSON_OBJECT() WHERE id=2",
				"DELETE FROM evaluation_outcome WHERE id IN (0,2,18446744073709551615)",
				"INSERT INTO evaluation_outcome VALUES(1,NULL)",
			} {
				if _, err := admin.ExecContext(ctx, statement); err != nil {
					t.Error("owned_snapshot_fact_restore_failed")
				}
			}
		}()
		values := make([]string, 0, 1001)
		for id := 1000; id <= 2000; id++ {
			values = append(values, "("+strconv.Itoa(id)+",NULL)")
		}
		if _, err := admin.ExecContext(ctx, "INSERT INTO assessment VALUES"+strings.Join(values, ",")); err != nil {
			t.Fatal("owned_multiple_page_setup_failed")
		}
		db := open("readiness_global")
		defer func() {
			if db.Close() != nil {
				t.Error("owned_snapshot_pool_release_failed")
			}
		}()
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatal("owned_snapshot_connection_failed")
		}
		defer func() {
			if c.Close() != nil {
				t.Error("owned_snapshot_connection_release_failed")
			}
		}()
		snapshot, stop := context.WithTimeout(ctx, totalLimit)
		defer stop()
		r := baseReadinessReceipt(b)
		if readinessControl(snapshot, c, &r, readinessIsolationSQL) != nil || readinessControl(snapshot, c, &r, readinessBeginSQL) != nil {
			t.Fatal("owned_readonly_rr_begin_failed")
		}
		active := true
		defer func() {
			if active && readinessControl(snapshot, c, &r, readinessRollbackSQL) != nil {
				t.Error("owned_snapshot_abort_failed")
			}
		}()
		upper, err := readinessRows(snapshot, c, readinessUpperSQL, 2)
		if err != nil || len(upper) != 1 || upper[0][0].String != "18446744073709551615" || upper[0][1].String != "18446744073709551615" {
			t.Fatal("owned_snapshot_anchor_failed")
		}
		columns, err := readinessRows(snapshot, c, readinessPrimaryColumnsSQL, 6)
		if err != nil {
			t.Fatal("owned_snapshot_column_guard_failed")
		}
		indexes, err := readinessRows(snapshot, c, readinessPrimaryIndexesSQL, 8)
		if err != nil || !readinessPrimaryShape(columns, indexes) {
			t.Fatal("owned_snapshot_primary_guard_failed")
		}
		for _, statement := range []string{
			"UPDATE assessment SET historical_lifecycle_evidence=JSON_OBJECT('fixture','changed-private-value') WHERE id=2",
			"DELETE FROM assessment WHERE id=1", "INSERT INTO assessment VALUES(4,NULL)",
			"UPDATE evaluation_outcome SET historical_committed_evidence=NULL WHERE id=0",
			"DELETE FROM evaluation_outcome WHERE id=1", "INSERT INTO evaluation_outcome VALUES(2,JSON_OBJECT())",
		} {
			if _, err := admin.ExecContext(ctx, statement); err != nil {
				t.Fatal("owned_concurrent_fact_change_failed")
			}
		}
		a, ap, ae, err := readinessPartitionPages(snapshot, c, &r, false, upper[0][0])
		if err != nil {
			var failure *readinessQueryFailure
			if errors.As(err, &failure) && failure != nil && failure.number != nil {
				t.Fatalf("owned_assessment_snapshot_page_failed category=%s pages=%d eof=%t queries=%d mysql_number=%d", err.Error(), ap, ae, r.QueryCount, *failure.number)
			}
			t.Fatalf("owned_assessment_snapshot_page_failed category=%s pages=%d eof=%t queries=%d", err.Error(), ap, ae, r.QueryCount)
		}
		o, op, oe, err := readinessPartitionPages(snapshot, c, &r, true, upper[0][1])
		if err != nil {
			var failure *readinessQueryFailure
			if errors.As(err, &failure) && failure != nil && failure.number != nil {
				t.Fatalf("owned_outcome_snapshot_page_failed category=%s pages=%d eof=%t queries=%d mysql_number=%d", err.Error(), op, oe, r.QueryCount, *failure.number)
			}
			t.Fatalf("owned_outcome_snapshot_page_failed category=%s pages=%d eof=%t queries=%d", err.Error(), op, oe, r.QueryCount)
		}
		if a != (scalarPartition{Total: 1006, SQLNull: 1003, NonNull: 3, EmptyObject: 2, NonEmptyObject: 1}) || o != (scalarPartition{Total: 3, SQLNull: 2, NonNull: 1, EmptyObject: 1}) || ap != 3 || op != 2 || !ae || !oe {
			t.Fatal("concurrent_facts_changed_rr_snapshot")
		}
		if readinessControl(snapshot, c, &r, readinessRollbackSQL) != nil {
			t.Fatal("owned_snapshot_rollback_failed")
		}
		active = false
		var live int
		if c.QueryRowContext(snapshot, "SELECT /*+ MAX_EXECUTION_TIME(5000) */ COUNT(*) FROM assessment WHERE id=4").Scan(&live) != nil || live != 1 {
			t.Fatal("owned_rollback_did_not_return_live_view")
		}
		if r.ControlStatementCount != 3 {
			t.Fatal("owned_snapshot_control_count_changed")
		}
	})
	t.Run("actual_signed_primary_shape_rejected", func(t *testing.T) {
		if _, err := admin.ExecContext(ctx, "ALTER TABLE assessment MODIFY id BIGINT NOT NULL"); err != nil {
			t.Fatal("owned_primary_mutation_failed")
		}
		defer func() {
			if _, err := admin.ExecContext(ctx, "ALTER TABLE assessment MODIFY id BIGINT UNSIGNED NOT NULL"); err != nil {
				t.Error("owned_primary_restore_failed")
			}
		}()
		r := reader("readiness_global")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "metadata_invalid" || r.FailureStage != "primary_indexes" || r.TransactionReleased == nil || !*r.TransactionReleased {
			t.Fatal("native_signed_primary_shape_accepted")
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
