package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func inventoryIdentityFixture(id uint64, version string) *sqlmock.Rows {
	return readinessRowsFixture(5, []any{id, fixtureUUID, fixtureConnection.Database, version, "NONE"})
}

func inventoryObjectsFixture() *sqlmock.Rows {
	var rows [][]any
	for _, table := range inventoryTables {
		rows = append(rows, []any{table, "BASE TABLE", "InnoDB"})
	}
	return readinessRowsFixture(3, rows...)
}

func inventoryPrefix(m sqlmock.Sqlmock) {
	m.ExpectQuery(inventoryIdentitySQL).WillReturnRows(inventoryIdentityFixture(10, "8.0.36"))
	m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{100, false}))
	m.ExpectExec(readinessIsolationSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectExec(readinessBeginSQL).WillReturnResult(sqlmock.NewResult(0, 0))
}

func inventoryBusinessExpectations(m sqlmock.Sqlmock) {
	m.ExpectQuery(inventoryObjectsPresentSQL).WillReturnRows(inventoryObjectsFixture())
	m.ExpectQuery(inventoryTesteeTagsPartitionSQL).WillReturnRows(readinessRowsFixture(7, []any{4, 1, 3, 1, 1, 1, 0}))
	m.ExpectQuery(inventoryOperatorsRoleDuplicationSQL).WillReturnRows(readinessRowsFixture(6, []any{3, 0, 0, 3, 2, 1}))
	m.ExpectQuery(inventoryFiveTableCountsSQL).WillReturnRows(readinessRowsFixture(2, []any{"testee_store_migration_manifests", 2}, []any{"testee_store_migration_items", 3}, []any{"testee_store_migration_reversions", 1}, []any{"operator_retirement_manifests", 3}, []any{"clinician_operator_binding_archive", 2}))
	m.ExpectQuery(inventoryTesteeManifestStatesSQL).WillReturnRows(readinessRowsFixture(2, []any{"applied", 1}, []any{"unknown", 1}))
	m.ExpectQuery(inventoryTesteeItemDispositionsSQL).WillReturnRows(readinessRowsFixture(2, []any{"candidate", 1}, []any{"deferred", 1}, []any{"already_assigned", 1}))
	m.ExpectQuery(inventoryOperatorManifestStagesSQL).WillReturnRows(readinessRowsFixture(2, []any{"executing", 1}, []any{"applied_unchanged", 1}, []any{"unknown", 1}))
	m.ExpectQuery(inventoryHistoricalReferenceGapsSQL).WillReturnRows(readinessRowsFixture(3, []any{1, 1, 1}))
	m.ExpectQuery(inventoryAIBackfillScopeSQL).WillReturnRows(readinessRowsFixture(8, []any{4, 1, 1, 1, 3, 1, 1, 2}))
	m.ExpectQuery(inventoryAIExpectedRelationGapsSQL).WillReturnRows(readinessRowsFixture(3, []any{5, 1, 2}))
	m.ExpectQuery(inventoryAIActualRelationGapsSQL).WillReturnRows(readinessRowsFixture(4, []any{5, 1, 1, 2}))
}

func inventoryFinish(m sqlmock.Sqlmock, version uint64, id uint64) {
	m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{version, false}))
	if version == 100 {
		m.ExpectQuery(inventoryIdentitySQL).WillReturnRows(inventoryIdentityFixture(id, "8.0.36"))
	}
}

func assertInventoryPrivate(t *testing.T, r inventoryReceipt) {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal("inventory_receipt_marshal_failed")
	}
	for _, bad := range []string{fixtureSecret, fixtureUUID, fixtureConnection.Host, fixtureConnection.User, "CURRENT_ROLE", "SELECT ", "org_id", "request_id"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("inventory_private_data_leaked")
		}
	}
	if !r.InventoryOnly || r.FreezeVerified || (!r.Complete && r.Measurements != nil) {
		t.Fatal("inventory_incomplete_or_authority_misrepresented")
	}
}

func TestRetirementInventoryCountsFindingsWithoutEligibilityAndReleases(t *testing.T) {
	db, m := readinessMock(t)
	inventoryPrefix(m)
	inventoryBusinessExpectations(m)
	inventoryFinish(m, 100, 10)
	r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.Complete || r.Measurements == nil {
		t.Fatal("inventory_completed_before_pool_release")
	}
	m.ExpectClose()
	r = releaseInventoryDatabase(db, r)
	if !r.Complete || r.QueryCount != 15 || r.ControlStatements != 3 || r.Measurements.MySQLVersion != (inventoryServerVersion{8, 0, 36}) || r.Measurements.Counts["operator_manifest_stages"]["unknown"] != 1 || r.Measurements.Counts["operators_role_duplication"]["different_canonical_json_bytes"] != 1 || r.Measurements.Counts["ai_backfill_scope"]["indexed_identity_mismatch"] != 2 {
		t.Fatal("inventory_findings_or_exact_budget_lost")
	}
	assertInventoryPrivate(t, r)
	if m.ExpectationsWereMet() != nil {
		t.Fatal("inventory_did_not_use_one_fixed_read_sequence")
	}
}

func TestRetirementInventoryRejectsMissingOrNonInnoDBObjectsBeforeCounts(t *testing.T) {
	for _, bad := range []string{"missing", "engine"} {
		t.Run(bad, func(t *testing.T) {
			db, m := readinessMock(t)
			inventoryPrefix(m)
			var rows *sqlmock.Rows
			if bad == "missing" {
				rows = readinessRowsFixture(3)
			} else {
				rows = readinessRowsFixture(3)
				for i, table := range inventoryTables {
					engine := "InnoDB"
					if i == 0 {
						engine = "MyISAM"
					}
					rows.AddRow(table, "BASE TABLE", engine)
				}
			}
			m.ExpectQuery(inventoryObjectsPresentSQL).WillReturnRows(rows)
			m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
			r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
			m.ExpectClose()
			r = releaseInventoryDatabase(db, r)
			if r.ErrorCategory != "object_shape_rejected" || r.QueryCount != 3 || r.TransactionReleased == nil || !*r.TransactionReleased {
				t.Fatal("object_visibility_or_engine_became_zero")
			}
			assertInventoryPrivate(t, r)
		})
	}
}

func TestRetirementInventoryUnknownCountsRemainBoundedAndConserve(t *testing.T) {
	for _, rows := range [][][]sql.NullString{{{{String: "injected", Valid: true}, {String: "1", Valid: true}}}, {{{String: "unknown", Valid: true}, {String: "1", Valid: true}}, {{String: "unknown", Valid: true}, {String: "1", Valid: true}}}, {{{String: "unknown", Valid: true}, {String: "-1", Valid: true}}}} {
		if _, ok := inventoryGroupedCounts(rows, []string{"applied", "rolled_back", "unknown"}, false); ok {
			t.Fatal("unknown_raw_label_duplicate_or_negative_accepted")
		}
	}
	if inventorySumEquals(map[string]uint64{"a": ^uint64(0), "b": 1}, 0, "a", "b") {
		t.Fatal("count_overflow_became_zero")
	}
}

func TestRetirementInventoryPostRollbackChecksAreLive(t *testing.T) {
	for _, changed := range []string{"version", "identity"} {
		t.Run(changed, func(t *testing.T) {
			db, m := readinessMock(t)
			inventoryPrefix(m)
			inventoryBusinessExpectations(m)
			version, id := uint64(100), uint64(10)
			if changed == "version" {
				version = 101
			} else {
				id = 11
			}
			inventoryFinish(m, version, id)
			r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
			m.ExpectClose()
			r = releaseInventoryDatabase(db, r)
			want := "migration_pair_rejected"
			if changed == "identity" {
				want = "session_identity_changed"
			}
			if r.ErrorCategory != want || r.TransactionReleased == nil || !*r.TransactionReleased {
				t.Fatal("snapshot_hid_live_final_change")
			}
			assertInventoryPrivate(t, r)
		})
	}
}

func TestRetirementInventoryReleaseFailureDiscardsAllCounts(t *testing.T) {
	for _, release := range []string{"transaction", "database"} {
		t.Run(release, func(t *testing.T) {
			db, m := readinessMock(t)
			inventoryPrefix(m)
			inventoryBusinessExpectations(m)
			if release == "transaction" {
				m.ExpectExec(readinessRollbackSQL).WillReturnError(errors.New(fixtureSecret))
			} else {
				inventoryFinish(m, 100, 10)
			}
			r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
			if release == "database" {
				m.ExpectClose().WillReturnError(errors.New(fixtureSecret))
			} else {
				m.ExpectClose()
			}
			r = releaseInventoryDatabase(db, r)
			want := "transaction_release_failed"
			if release == "database" {
				want = "database_release_failed"
			}
			if r.ErrorCategory != want || r.Measurements != nil {
				t.Fatal("failed_release_retained_success")
			}
			assertInventoryPrivate(t, r)
		})
	}
}

func TestRetirementInventoryVersionAndPublicModeAreClosed(t *testing.T) {
	for _, version := range []string{"8.0", "8.0.36secret", "08.0.36", "8.0.36-", "10.6.1-MariaDB", "5.7.45"} {
		if _, ok := inventoryVersion(version); ok {
			t.Fatal("unrecognized_server_version_accepted")
		}
	}
	for _, version := range []string{"8.0.36", "8.4.11", "8.0.36-commercial"} {
		if _, ok := inventoryVersion(version); !ok {
			t.Fatal("valid_numeric_mysql_prefix_rejected")
		}
	}
	var out bytes.Buffer
	if run([]string{"--retirement-inventory"}, func(string) string { return "" }, &out) == 0 {
		t.Fatal("inventory_without_binding_accepted")
	}
	var value inventoryReceipt
	if json.Unmarshal(out.Bytes(), &value) != nil || !value.InventoryOnly || value.QueryCount != 0 || value.Complete {
		t.Fatal("inventory_mode_fell_into_default_profile")
	}
}

func TestRetirementInventoryWholeBudgetIncludesPoolRelease(t *testing.T) {
	db, m := readinessMock(t)
	inventoryPrefix(m)
	inventoryBusinessExpectations(m)
	inventoryFinish(m, 100, 10)
	r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
	r.deadline = time.Now().Add(-time.Nanosecond)
	m.ExpectClose()
	r = releaseInventoryDatabase(db, r)
	if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_deadline_exceeded" {
		t.Fatal("finished_queries_reset_the_whole_budget")
	}
	assertInventoryPrivate(t, r)
}

func TestRetirementInventoryFailedOrTruncatedReadNeverBecomesZero(t *testing.T) {
	for _, failure := range []string{"permission", "iteration", "truncated"} {
		t.Run(failure, func(t *testing.T) {
			db, m := readinessMock(t)
			inventoryPrefix(m)
			query := m.ExpectQuery(inventoryObjectsPresentSQL)
			want := "query_permission_denied"
			switch failure {
			case "permission":
				query.WillReturnError(&mysql.MySQLError{Number: 1142, Message: fixtureSecret})
			case "iteration":
				query.WillReturnRows(inventoryObjectsFixture().RowError(2, &mysql.MySQLError{Number: 1142, Message: fixtureSecret}))
			case "truncated":
				rows := readinessRowsFixture(3)
				for range 1001 {
					rows.AddRow("testee", "BASE TABLE", "InnoDB")
				}
				query.WillReturnRows(rows)
				want = "metadata_truncated"
			}
			m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
			r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
			m.ExpectClose()
			r = releaseInventoryDatabase(db, r)
			if r.ErrorCategory != want || r.Measurements != nil || r.QueryCount != 3 || r.TransactionReleased == nil || !*r.TransactionReleased || r.ConnectionReleased == nil || !*r.ConnectionReleased {
				t.Fatal("partial_read_was_not_discarded_or_released")
			}
			if failure != "truncated" && (r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1142) {
				t.Fatal("typed_failure_number_not_preserved")
			}
			assertInventoryPrivate(t, r)
			if m.ExpectationsWereMet() != nil {
				t.Fatal("failed_read_continued_business_observation")
			}
		})
	}
}

func TestRetirementInventoryConservationIncludesCrossFamilyTotals(t *testing.T) {
	db, m := readinessMock(t)
	inventoryPrefix(m)
	inventoryBusinessExpectations(m)
	inventoryFinish(m, 100, 10)
	r := probeInventory(context.Background(), db, fixtureConnection, fixtureBinding())
	m.ExpectClose()
	r = releaseInventoryDatabase(db, r)
	if !r.Complete {
		t.Fatal("conservation_fixture_failed")
	}
	for _, change := range []struct{ family, field string }{
		{"five_table_counts", "testee_store_migration_manifests"},
		{"operators_role_duplication", "total"},
		{"ai_backfill_scope", "indexed_identity_mismatch"},
		{"ai_expected_relation_gaps", "invalid_expected_id_entries"},
		{"ai_actual_relation_gaps", "orphan_relations"},
	} {
		values := r.Measurements.Counts[change.family]
		before := values[change.field]
		values[change.field] = ^uint64(0)
		if inventoryConserves(r.Measurements.Counts) {
			t.Fatal("inconsistent_full_read_was_accepted")
		}
		values[change.field] = before
	}
}
