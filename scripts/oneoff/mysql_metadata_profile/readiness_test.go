package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func readinessIdentities(id uint64, roles string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "uuid", "database", "version", "roles", "performance_schema", "event_scheduler"}).AddRow(id, fixtureUUID, fixtureConnection.Database, "8.0.36", roles, 1, "ON")
}
func toDriverValues(values []any) []driver.Value {
	result := make([]driver.Value, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}
func readinessRowsFixture(width int, values ...[]any) *sqlmock.Rows {
	columns := make([]string, width)
	for i := range columns {
		columns[i] = fmt.Sprintf("c%d", i)
	}
	rows := sqlmock.NewRows(columns)
	for _, row := range values {
		converted := make([]interface{}, len(row))
		copy(converted, row)
		rows.AddRow(toDriverValues(converted)...)
	}
	return rows
}
func readinessMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, m := mockDB(t)
	db.SetMaxIdleConns(1)
	return db, m
}
func readinessPrefix(m sqlmock.Sqlmock) {
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{100, false}))
}
func readinessCatalogExpectations(m sqlmock.Sqlmock) {
	m.ExpectQuery(readinessTablesSQL).WillReturnRows(readinessRowsFixture(3, []any{"assessment", "BASE TABLE", "InnoDB"}, []any{"evaluation_outcome", "BASE TABLE", "InnoDB"}, []any{"schema_migrations", "BASE TABLE", "InnoDB"}))
	m.ExpectQuery(readinessViewsSQL).WillReturnRows(readinessRowsFixture(3))
	m.ExpectQuery(readinessRoutinesSQL).WillReturnRows(readinessRowsFixture(3))
	m.ExpectQuery(readinessTriggersSQL).WillReturnRows(readinessRowsFixture(4))
	m.ExpectQuery(readinessEventsSQL).WillReturnRows(readinessRowsFixture(5, []any{"another_schema", fixtureSecret, "ENABLED", "RECURRING", nil}))
	m.ExpectQuery(readinessColumnsSQL).WillReturnRows(readinessRowsFixture(4, []any{"assessment", "historical_lifecycle_evidence", "json", "YES"}, []any{"evaluation_outcome", "historical_committed_evidence", "json", "YES"}))
	m.ExpectQuery(readinessChecksSQL).WillReturnRows(readinessRowsFixture(4, []any{"assessment", "chk_assessment_historical_lifecycle", "CHECK", "YES"}, []any{"evaluation_outcome", "chk_outcome_historical_committed", "CHECK", "YES"}))
}
func readinessOperationExpectations(m sqlmock.Sqlmock) {
	m.ExpectQuery(readinessThreadsSQL).WillReturnRows(readinessRowsFixture(5, []any{15, "thread/sql/one_connection", "FOREGROUND", 16, "Sleep"}))
	m.ExpectQuery(readinessTransactionsSQL).WillReturnRows(readinessRowsFixture(6, []any{16, "RUNNING", "2026-10-14 01:02:03", 1, 2, false}))
	m.ExpectQuery(readinessLocksSQL).WillReturnRows(readinessRowsFixture(7, []any{"TABLE", fixtureConnection.Database, "assessment", "SHARED_WRITE", "TRANSACTION", "GRANTED", 15}))
	m.ExpectQuery(readinessInstrumentSQL).WillReturnRows(readinessRowsFixture(2, []any{"wait/lock/metadata/sql/mdl", "YES"}))
}
func readinessFinishExpectations(m sqlmock.Sqlmock) {
	m.ExpectQuery(readinessAssessmentSQL).WillReturnRows(readinessRowsFixture(7, []any{5, 1, 4, 1, 1, 1, 1}))
	m.ExpectQuery(readinessOutcomeSQL).WillReturnRows(readinessRowsFixture(7, []any{0, 0, 0, 0, 0, 0, 0}))
	m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{100, false}))
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
}
func assertReadinessPrivate(t *testing.T, r readinessReceipt) {
	t.Helper()
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixtureSecret, fixtureUUID, fixtureConnection.User, fixtureConnection.Database, "SHOW GRANTS", "another_schema", "NONE"} {
		if bytes.Contains(raw, []byte(bad)) {
			t.Fatal("private_readiness_value_leaked")
		}
	}
	if !r.Complete && r.Measurements != nil {
		t.Fatal("incomplete_readiness_retained_measurements")
	}
	if r.FreezeVerified || !r.DiagnosticOnly || !r.ReadinessOnly {
		t.Fatal("readiness_was_misnamed_freeze")
	}
}
func TestMetadataReadinessObservesNonzeroFactsAndCompletesOnlyAfterRelease(t *testing.T) {
	db, m := readinessMock(t)
	readinessPrefix(m)
	readinessCatalogExpectations(m)
	readinessOperationExpectations(m)
	readinessFinishExpectations(m)
	r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.Complete || r.ErrorCategory != "none" || r.Measurements == nil || r.Measurements.Transactions != 1 || r.Measurements.EnabledEvents != 1 || r.QueryCount != 18 {
		t.Fatalf("readiness observation failed: %s/%s", r.ErrorCategory, r.FailureStage)
	}
	m.ExpectClose()
	r = releaseReadinessDatabase(db, r)
	if !r.Complete || r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
		t.Fatal("release_completion_failed")
	}
	assertReadinessPrivate(t, r)
}
func TestMetadataReadinessMissingProcessDoesNotBecomeZero(t *testing.T) {
	db, m := readinessMock(t)
	readinessPrefix(m)
	readinessCatalogExpectations(m)
	m.ExpectQuery(readinessThreadsSQL).WillReturnRows(readinessRowsFixture(5))
	m.ExpectQuery(readinessTransactionsSQL).WillReturnError(&mysql.MySQLError{Number: 1227, Message: fixtureSecret})
	r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
	m.ExpectClose()
	r = releaseReadinessDatabase(db, r)
	if r.Complete || r.ErrorCategory != "query_failed" || r.FailureStage != "transactions" {
		t.Fatal("process_failure_became_empty_view")
	}
	assertReadinessPrivate(t, r)
}
func TestMetadataReadinessUnknownOrIncompleteGrantsStopBeforeCatalog(t *testing.T) {
	for _, row := range []string{"GRANT SELECT ON `fixture_db`.* TO `fixture_user`@`%`", positive() + "; injected", "REVOKE SELECT ON `hidden`.* FROM `fixture_user`@`%`"} {
		db, m := readinessMock(t)
		m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
		m.ExpectQuery(currentSQL).WillReturnRows(grants(row))
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.ErrorCategory != "grants_unverified" || r.QueryCount != 2 {
			t.Fatal("unverified_visibility_accepted")
		}
		assertReadinessPrivate(t, r)
	}
}
func TestMetadataReadinessMetadataCannotSilentlyTruncateOrLoseCursor(t *testing.T) {
	for _, tc := range []string{"sentinel", "cursor_error", "close_error", "field_limit", "query_timeout"} {
		t.Run(tc, func(t *testing.T) {
			db, m := readinessMock(t)
			readinessPrefix(m)
			q := m.ExpectQuery(readinessTablesSQL)
			rows := readinessRowsFixture(3)
			switch tc {
			case "sentinel":
				for i := 0; i < 1001; i++ {
					rows.AddRow(fmt.Sprintf("owned_%04d", i), "BASE TABLE", "InnoDB")
				}
				q.WillReturnRows(rows)
			case "cursor_error":
				rows.AddRow("one", "BASE TABLE", "InnoDB").RowError(0, errors.New(fixtureSecret))
				q.WillReturnRows(rows)
			case "close_error":
				rows.AddRow("one", "BASE TABLE", "InnoDB").CloseError(errors.New(fixtureSecret))
				q.WillReturnRows(rows)
			case "field_limit":
				rows.AddRow(strings.Repeat("x", 65537), "BASE TABLE", "InnoDB")
				q.WillReturnRows(rows)
			case "query_timeout":
				q.WillReturnError(context.DeadlineExceeded)
			}
			r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
			m.ExpectClose()
			r = releaseReadinessDatabase(db, r)
			if r.Complete || r.FailureStage != "tables" || r.ErrorCategory == "none" {
				t.Fatal("partial_catalog_accepted")
			}
			assertReadinessPrivate(t, r)
		})
	}
}
func TestMetadataReadinessDisabledPerformanceSchemaDoesNotShowEmptySuccess(t *testing.T) {
	db, m := readinessMock(t)
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(sqlmock.NewRows([]string{"id", "uuid", "db", "version", "roles", "pfs", "scheduler"}).AddRow(10, fixtureUUID, fixtureConnection.Database, "8.0.36", "NONE", 0, "ON"))
	r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
	m.ExpectClose()
	r = releaseReadinessDatabase(db, r)
	if r.ErrorCategory != "storage_observation_unavailable" || r.QueryCount != 1 {
		t.Fatal("disabled_pfs_became_empty_view")
	}
	assertReadinessPrivate(t, r)
}
func TestMetadataReadinessDisabledInstrumentIsUnknown(t *testing.T) {
	db, m := readinessMock(t)
	readinessPrefix(m)
	readinessCatalogExpectations(m)
	m.ExpectQuery(readinessThreadsSQL).WillReturnRows(readinessRowsFixture(5))
	m.ExpectQuery(readinessTransactionsSQL).WillReturnRows(readinessRowsFixture(6))
	m.ExpectQuery(readinessLocksSQL).WillReturnRows(readinessRowsFixture(7))
	m.ExpectQuery(readinessInstrumentSQL).WillReturnRows(readinessRowsFixture(2, []any{"wait/lock/metadata/sql/mdl", "NO"}))
	r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
	m.ExpectClose()
	r = releaseReadinessDatabase(db, r)
	if r.ErrorCategory != "storage_observation_unavailable" || r.FailureStage != "instrument" {
		t.Fatal("disabled_mdl_became_empty_view")
	}
	assertReadinessPrivate(t, r)
}
func TestMetadataReadinessRejectsDirtyOrUnexpectedVersion(t *testing.T) {
	for _, value := range [][]any{{100, true}, {101, false}, {99, false}} {
		db, m := readinessMock(t)
		m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
		m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
		m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, value))
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.ErrorCategory != "migration_pair_rejected" {
			t.Fatal("unexpected_migration_accepted")
		}
		assertReadinessPrivate(t, r)
	}
}
func TestMetadataReadinessFinalIdentityDiscardsMeasurements(t *testing.T) {
	for _, roles := range []string{"NONE", "`changed`@`%`"} {
		db, m := readinessMock(t)
		readinessPrefix(m)
		readinessCatalogExpectations(m)
		readinessOperationExpectations(m)
		m.ExpectQuery(readinessAssessmentSQL).WillReturnRows(readinessRowsFixture(7, []any{0, 0, 0, 0, 0, 0, 0}))
		m.ExpectQuery(readinessOutcomeSQL).WillReturnRows(readinessRowsFixture(7, []any{0, 0, 0, 0, 0, 0, 0}))
		m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{100, false}))
		m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(11, roles))
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.ErrorCategory != "session_identity_changed" {
			t.Fatal("changed_identity_accepted")
		}
		assertReadinessPrivate(t, r)
	}
}
func TestMetadataReadinessReleaseFailuresRevokeMeasurements(t *testing.T) {
	t.Run("pool_driver_close", func(t *testing.T) {
		db, m := readinessMock(t)
		readinessPrefix(m)
		readinessCatalogExpectations(m)
		readinessOperationExpectations(m)
		readinessFinishExpectations(m)
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		m.ExpectClose().WillReturnError(errors.New(fixtureSecret))
		r = releaseReadinessDatabase(db, r)
		if r.ErrorCategory != "database_release_failed" || r.DatabaseReleased == nil || *r.DatabaseReleased {
			t.Fatal("pool_close_error_accepted")
		}
		assertReadinessPrivate(t, r)
	})
	t.Run("pinned_connection", func(t *testing.T) {
		db, m := readinessMock(t)
		c, e := db.Conn(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if c.Close() != nil {
			t.Fatal("fixture_close_failed")
		}
		r := baseReadinessReceipt(fixtureBinding())
		r.Measurements = &readinessMeasurements{}
		r.ErrorCategory = "none"
		r = releaseReadinessConnection(c, r)
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.ErrorCategory != "connection_release_failed" {
			t.Fatal("connection_release_error_accepted")
		}
		assertReadinessPrivate(t, r)
	})
}
func TestMetadataReadinessScalarPartitionCannotOverflowOrInventZero(t *testing.T) {
	for _, value := range [][]any{{5, 1, 4, 1, 1, 1, 1}, {0, 0, 0, 0, 0, 0, 0}, {5, 1, 4, 1, 1, 1, 0}, {"18446744073709551615", 0, "18446744073709551615", "18446744073709551615", 1, 0, 0}, {0, nil, 0, 0, 0, 0, 0}, {0, -1, 0, 0, 0, 0, 0}} {
		rows := make([]sql.NullString, 7)
		for i, v := range value {
			if v != nil {
				rows[i] = sql.NullString{String: fmt.Sprint(v), Valid: true}
			}
		}
		_, ok := readScalarPartition([][]sql.NullString{rows})
		want := fmt.Sprint(value) == "[5 1 4 1 1 1 1]" || fmt.Sprint(value) == "[0 0 0 0 0 0 0]"
		if ok != want {
			t.Fatal("scalar_partition_not_conserved")
		}
	}
}
func TestMetadataReadinessPublicInputRequiresQSWithoutConnecting(t *testing.T) {
	env := map[string]string{"MYSQL_HOST": fixtureConnection.Host, "MYSQL_PORT": "3306", "MYSQL_DATABASE": "fixture_db", "MYSQL_USERNAME": "fixture_user", "MYSQL_PASSWORD": fixtureSecret, "PROFILE_SOURCE_SHA": strings.Repeat("a", 40), "PROFILE_RUN_ID": "12345-1", "PROFILE_EXPECTED_TARGET_HASH": strings.Repeat("0", 64)}
	var out bytes.Buffer
	if run([]string{"--metadata-readiness"}, func(k string) string { return env[k] }, &out) != 1 {
		t.Fatal("non_qs_readiness_accepted")
	}
	var r readinessReceipt
	if json.Unmarshal(out.Bytes(), &r) != nil || r.ErrorCategory != "connection_input_invalid" || r.QueryCount != 0 || r.ConnectionReleased != nil || r.DatabaseReleased != nil {
		t.Fatal("input_guard_not_early")
	}
	assertReadinessPrivate(t, r)
}
