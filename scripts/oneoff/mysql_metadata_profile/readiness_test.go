package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

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
func readinessPrimaryExpectations(m sqlmock.Sqlmock) {
	m.ExpectQuery(readinessPrimaryColumnsSQL).WillReturnRows(readinessRowsFixture(6,
		[]any{"assessment", "id", "bigint", "bigint unsigned", "NO", "InnoDB"},
		[]any{"evaluation_outcome", "id", "bigint", "bigint unsigned", "NO", "InnoDB"}))
	m.ExpectQuery(readinessPrimaryIndexesSQL).WillReturnRows(readinessRowsFixture(8,
		[]any{"assessment", 1, "id", 0, "BTREE", nil, nil, "YES"},
		[]any{"evaluation_outcome", 1, "id", 0, "BTREE", nil, nil, "YES"}))
}
func readinessSnapshotExpectations(m sqlmock.Sqlmock, assessment, outcome any) {
	m.ExpectExec(readinessIsolationSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectExec(readinessBeginSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery(readinessUpperSQL).WillReturnRows(readinessRowsFixture(2, []any{assessment, outcome}))
	readinessPrimaryExpectations(m)
}
func readinessEmptyPagesExpectations(m sqlmock.Sqlmock) {
	readinessSnapshotExpectations(m, nil, nil)
	m.ExpectQuery(readinessPageSQL(false, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2))
	m.ExpectQuery(readinessPageSQL(true, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2))
	m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
}
func readinessFinishExpectations(m sqlmock.Sqlmock) {
	readinessSnapshotExpectations(m, 4, nil)
	m.ExpectQuery(readinessPageSQL(false, false)).WithArgs("4").WillReturnRows(readinessRowsFixture(2, []any{0, 0}, []any{1, 1}, []any{2, 2}, []any{3, 3}, []any{4, 4}))
	m.ExpectQuery(readinessPageSQL(false, true)).WithArgs("4", "4").WillReturnRows(readinessRowsFixture(2))
	m.ExpectQuery(readinessPageSQL(true, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2))
	m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
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
	if r.Complete || r.ErrorCategory != "none" || r.Measurements == nil || r.Measurements.Transactions != 1 || r.Measurements.EnabledEvents != 1 || r.QueryCount != 22 || r.ControlStatementCount != 3 || r.TransactionReleased == nil || !*r.TransactionReleased || r.FormatVersion != 2 || !r.Measurements.PrimaryKeyShapeVerified || !r.Measurements.ConsistentSnapshot || !r.Measurements.AssessmentEOF || !r.Measurements.OutcomeEOF {
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
	if r.Complete || r.ErrorCategory != "query_permission_denied" || r.FailureStage != "transactions" {
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
		readinessEmptyPagesExpectations(m)
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
	cases := []struct {
		p    scalarPartition
		want bool
	}{
		{scalarPartition{Total: 5, SQLNull: 1, NonNull: 4, JSONNull: 1, EmptyObject: 1, NonEmptyObject: 1, Other: 1}, true},
		{scalarPartition{}, true},
		{scalarPartition{Total: 5, SQLNull: 1, NonNull: 4, JSONNull: 1, EmptyObject: 1, NonEmptyObject: 1}, false},
		{scalarPartition{Total: ^uint64(0), NonNull: ^uint64(0), JSONNull: ^uint64(0), EmptyObject: 1}, false},
		{scalarPartition{SQLNull: 1}, false},
	}
	for _, tc := range cases {
		if scalarPartitionConserved(tc.p) != tc.want {
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

func TestMetadataReadinessQueryErrorsRemainPrivateAndIncomplete(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category string
	}{
		{"server_timeout", &mysql.MySQLError{Number: 3024, Message: fixtureSecret}, "query_server_timeout"},
		{"server_interrupted", &mysql.MySQLError{Number: 1317, Message: fixtureSecret}, "query_server_interrupted"},
		{"table_permission", &mysql.MySQLError{Number: 1142, Message: fixtureSecret}, "query_permission_denied"},
		{"column_permission", &mysql.MySQLError{Number: 1143, Message: fixtureSecret}, "query_permission_denied"},
		{"process_permission", &mysql.MySQLError{Number: 1227, Message: fixtureSecret}, "query_permission_denied"},
		{"context_deadline", context.DeadlineExceeded, "query_deadline_exceeded"},
		{"context_cancel", context.Canceled, "query_canceled"},
		{"server_zero", &mysql.MySQLError{Number: 0, Message: fixtureSecret}, "query_mysql_error"},
		{"server_max", &mysql.MySQLError{Number: 65535, Message: fixtureSecret}, "query_mysql_error"},
		{"network_timeout", &net.OpError{Op: fixtureSecret, Net: fixtureSecret, Err: readinessPrivateNetworkError{timeout: true}}, "query_network_timeout"},
		{"network_error", &net.OpError{Op: fixtureSecret, Net: fixtureSecret, Err: readinessPrivateNetworkError{}}, "query_network_error"},
		{"invalid_connection", mysql.ErrInvalidConn, "query_invalid_connection"},
		{"bad_connection", driver.ErrBadConn, "query_bad_connection"},
		{"failure_eof", io.EOF, "query_eof"},
		{"failure_unexpected_eof", io.ErrUnexpectedEOF, "query_unexpected_eof"},
		{"unknown_server", &mysql.MySQLError{Number: 1205, Message: fixtureSecret + " 3024 timeout"}, "query_mysql_error"},
		{"text_is_not_typed_error", errors.New(fixtureSecret + " 3024 context deadline exceeded"), "query_failed"},
		{"server_before_context", errors.Join(&mysql.MySQLError{Number: 1142, Message: fixtureSecret}, context.DeadlineExceeded), "query_permission_denied"},
	}
	for _, tc := range cases {
		for _, phase := range []string{"query", "iteration"} {
			t.Run(tc.name+"/"+phase, func(t *testing.T) {
				db, m := readinessMock(t)
				readinessPrefix(m)
				readinessCatalogExpectations(m)
				readinessOperationExpectations(m)
				readinessSnapshotExpectations(m, nil, nil)
				m.ExpectQuery(readinessPageSQL(false, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2))
				q := m.ExpectQuery(readinessPageSQL(true, false)).WithArgs("0")
				private := fmt.Errorf("%s: %w", fixtureSecret, tc.err)
				if phase == "query" {
					q.WillReturnError(private)
				} else {
					q.WillReturnRows(readinessRowsFixture(2, []any{0, 0}).RowError(0, private))
				}
				if tc.name == "bad_connection" && phase == "query" {
					m.ExpectClose()
				} else {
					m.ExpectExec(readinessRollbackSQL).WillReturnResult(sqlmock.NewResult(0, 0))
					if tc.name == "bad_connection" {
						m.ExpectClose()
					}
				}
				r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
				if tc.name != "bad_connection" {
					m.ExpectClose()
				}
				r = releaseReadinessDatabase(db, r)
				expectedCategory, expectedStage := tc.category, "outcome_distribution"
				badQuery := tc.name == "bad_connection" && phase == "query"
				if badQuery {
					expectedCategory, expectedStage = "connection_release_failed", "connection_release"
				}
				if r.Complete || r.Measurements != nil || r.ErrorCategory != expectedCategory || r.FailureStage != expectedStage || r.QueryCount != 19 {
					t.Fatalf("query error classification failed: %s/%s/%d", r.ErrorCategory, r.FailureStage, r.QueryCount)
				}
				var server *mysql.MySQLError
				if errors.As(tc.err, &server) && server != nil {
					if r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != server.Number {
						t.Fatal("typed_mysql_number_lost")
					}
				} else if r.QueryMySQLNumber != nil {
					t.Fatal("non_server_error_gained_number")
				}
				if r.ConnectionReleased == nil || *r.ConnectionReleased != !badQuery || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
					t.Fatal("classified_failure_release_unverified")
				}
				assertReadinessPrivate(t, r)
			})
		}
	}
}

type readinessPrivateNetworkError struct{ timeout bool }

func (e readinessPrivateNetworkError) Error() string   { return fixtureSecret }
func (e readinessPrivateNetworkError) Timeout() bool   { return e.timeout }
func (e readinessPrivateNetworkError) Temporary() bool { return false }
func TestMetadataReadinessServerNumberSurvivesReleaseFailure(t *testing.T) {
	err := readinessQueryError(&mysql.MySQLError{Number: 1064, Message: fixtureSecret})
	r := invalidateReadinessQuery(baseReadinessReceipt(fixtureBinding()), err, "outcome_distribution")
	r.QueryCount = 16
	for _, release := range []struct{ category, stage string }{{"metadata_release_failed", "outcome_distribution"}, {"connection_release_failed", "connection_release"}, {"database_release_failed", "database_release"}} {
		failed := invalidateReadiness(r, release.category, release.stage)
		if failed.QueryMySQLNumber == nil || *failed.QueryMySQLNumber != 1064 {
			t.Fatal("observed_number_lost_on_release_failure")
		}
		assertReadinessPrivate(t, failed)
	}
	if readinessQueryError(driver.ErrBadConn).Error() != "query_bad_connection" {
		t.Fatal("bad_connection_classification_failed")
	}
}

func TestMetadataReadinessPrimaryShapeRequiresUnsignedSingleInnoDBKey(t *testing.T) {
	for _, field := range []string{"valid", "signed", "engine", "nullable", "composite", "expression", "invisible"} {
		t.Run(field, func(t *testing.T) {
			columns := [][]sql.NullString{}
			indexes := [][]sql.NullString{}
			for _, name := range []string{"assessment", "evaluation_outcome"} {
				c := []sql.NullString{}
				for _, v := range []string{name, "id", "bigint", "bigint unsigned", "NO", "InnoDB"} {
					c = append(c, sql.NullString{String: v, Valid: true})
				}
				columns = append(columns, c)
				k := []sql.NullString{}
				for _, v := range []string{name, "1", "id", "0", "BTREE", "", "", "YES"} {
					k = append(k, sql.NullString{String: v, Valid: v != ""})
				}
				indexes = append(indexes, k)
			}
			switch field {
			case "signed":
				columns[0][3].String = "bigint"
			case "engine":
				columns[1][5].String = "MyISAM"
			case "nullable":
				columns[0][4].String = "YES"
			case "composite":
				indexes = append(indexes, indexes[1])
			case "expression":
				indexes[0][6] = sql.NullString{String: fixtureSecret, Valid: true}
			case "invisible":
				indexes[1][7].String = "NO"
			}
			if readinessPrimaryShape(columns, indexes) != (field == "valid") {
				t.Fatal("unsafe_primary_shape_accepted")
			}
		})
	}
}
func TestMetadataReadinessPagesUseAllUnsignedKeysAndRequireTerminatingEOF(t *testing.T) {
	for _, name := range []string{"zero", "uintmax", "missing_upper", "bad_class", "duplicate", "short_then_more", "eof_error"} {
		t.Run(name, func(t *testing.T) {
			db, m := readinessMock(t)
			c, e := db.Conn(context.Background())
			if e != nil {
				t.Fatal("fixture_connection_failed")
			}
			high := "0"
			rows := readinessRowsFixture(2, []any{"0", "0"})
			if name == "uintmax" {
				high = "18446744073709551615"
				rows = readinessRowsFixture(2, []any{high, "0"})
			}
			switch name {
			case "missing_upper":
				high = "1"
			case "bad_class":
				rows = readinessRowsFixture(2, []any{0, 5})
			case "duplicate":
				rows = readinessRowsFixture(2, []any{0, 0}, []any{0, 0})
			}
			m.ExpectQuery(readinessPageSQL(false, false)).WithArgs(high).WillReturnRows(rows)
			if name == "zero" || name == "uintmax" || name == "missing_upper" || name == "short_then_more" || name == "eof_error" {
				q := m.ExpectQuery(readinessPageSQL(false, true)).WithArgs(high, high)
				if name == "missing_upper" { // Last observed key was zero, below the fixed upper bound.
					// Amend this query's exact argument to the actual last key.
					q.WithArgs("0", high)
				}
				switch name {
				case "short_then_more":
					q.WillReturnRows(readinessRowsFixture(2, []any{0, 0}))
				case "eof_error":
					q.WillReturnError(io.EOF)
				default:
					q.WillReturnRows(readinessRowsFixture(2))
				}
			}
			r := baseReadinessReceipt(fixtureBinding())
			p, pages, eof, err := readinessPartitionPages(context.Background(), c, &r, false, sql.NullString{String: high, Valid: true})
			if name == "zero" || name == "uintmax" {
				if err != nil || !eof || p.Total != 1 || p.SQLNull != 1 || pages != 2 {
					t.Fatal("unsigned_key_coverage_failed")
				}
			} else if err == nil || eof || p != (scalarPartition{}) {
				t.Fatal("partial_pages_retained")
			}
			if c.Close() != nil {
				t.Fatal("fixture_release_failed")
			}
			m.ExpectClose()
			if db.Close() != nil {
				t.Fatal("fixture_pool_release_failed")
			}
		})
	}
}
func TestMetadataReadinessPageLimitCannotBecomeComplete(t *testing.T) {
	db, m := readinessMock(t)
	c, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal("fixture_connection_failed")
	}
	const high = readinessNonEmptyPageLimit * readinessPageSize
	for page := 0; page <= readinessNonEmptyPageLimit; page++ {
		q := m.ExpectQuery(readinessPageSQL(false, page != 0))
		if page == 0 {
			q.WithArgs(strconv.Itoa(high))
		} else {
			q.WithArgs(strconv.Itoa(page*readinessPageSize-1), strconv.Itoa(high))
		}
		rows := readinessRowsFixture(2)
		limit := readinessPageSize
		if page == readinessNonEmptyPageLimit {
			limit = 1
		}
		for row := 0; row < limit; row++ {
			rows.AddRow(page*readinessPageSize+row, 0)
		}
		q.WillReturnRows(rows)
	}
	r := baseReadinessReceipt(fixtureBinding())
	p, pages, eof, err := readinessPartitionPages(context.Background(), c, &r, false, sql.NullString{String: strconv.Itoa(high), Valid: true})
	if err == nil || err.Error() != "pagination_limit_exceeded" || eof || p != (scalarPartition{}) || pages != 513 || r.QueryCount != 513 {
		t.Fatal("pagination_limit_became_complete")
	}
	if c.Close() != nil {
		t.Fatal("fixture_release_failed")
	}
	m.ExpectClose()
	if db.Close() != nil {
		t.Fatal("fixture_pool_release_failed")
	}
}
func TestMetadataReadinessSnapshotFailureNeverRunsLiveChecks(t *testing.T) {
	for _, phase := range []string{"begin", "upper", "eof", "rollback", "eof_and_rollback"} {
		t.Run(phase, func(t *testing.T) {
			db, m := readinessMock(t)
			readinessPrefix(m)
			readinessCatalogExpectations(m)
			readinessOperationExpectations(m)
			m.ExpectExec(readinessIsolationSQL).WillReturnResult(sqlmock.NewResult(0, 0))
			if phase == "begin" {
				m.ExpectExec(readinessBeginSQL).WillReturnError(context.Canceled)
			} else {
				m.ExpectExec(readinessBeginSQL).WillReturnResult(sqlmock.NewResult(0, 0))
				if phase == "upper" {
					m.ExpectQuery(readinessUpperSQL).WillReturnError(context.DeadlineExceeded)
				} else {
					m.ExpectQuery(readinessUpperSQL).WillReturnRows(readinessRowsFixture(2, []any{0, nil}))
					readinessPrimaryExpectations(m)
					m.ExpectQuery(readinessPageSQL(false, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2, []any{0, 0}))
					if phase == "eof" || phase == "eof_and_rollback" {
						failure := context.DeadlineExceeded
						if phase == "eof_and_rollback" {
							failure = &mysql.MySQLError{Number: 1064, Message: fixtureSecret}
						}
						m.ExpectQuery(readinessPageSQL(false, true)).WithArgs("0", "0").WillReturnError(failure)
					} else {
						m.ExpectQuery(readinessPageSQL(false, true)).WithArgs("0", "0").WillReturnRows(readinessRowsFixture(2))
						m.ExpectQuery(readinessPageSQL(true, false)).WithArgs("0").WillReturnRows(readinessRowsFixture(2))
					}
				}
			}
			q := m.ExpectExec(readinessRollbackSQL)
			switch phase {
			case "eof_and_rollback":
				q.WillReturnError(context.DeadlineExceeded)
			case "rollback":
				q.WillReturnError(&mysql.MySQLError{Number: 1064, Message: fixtureSecret})
			default:
				q.WillReturnResult(sqlmock.NewResult(0, 0))
			}
			r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
			m.ExpectClose()
			r = releaseReadinessDatabase(db, r)
			if r.Complete || r.Measurements != nil || r.ControlStatementCount != 3 || r.TransactionReleased == nil || *r.TransactionReleased != (phase != "rollback" && phase != "eof_and_rollback") {
				t.Fatal("snapshot_failure_became_complete")
			}
			if (phase == "rollback" || phase == "eof_and_rollback") && (r.ErrorCategory != "transaction_release_failed" || r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1064) {
				t.Fatal("rollback_failure_not_preserved")
			}
			assertReadinessPrivate(t, r)
		})
	}
}
func TestMetadataReadinessPostRollbackVersionAndWholeDeadlineRemainLive(t *testing.T) {
	t.Run("version_changed_after_snapshot", func(t *testing.T) {
		db, m := readinessMock(t)
		readinessPrefix(m)
		readinessCatalogExpectations(m)
		readinessOperationExpectations(m)
		readinessEmptyPagesExpectations(m)
		m.ExpectQuery(readinessMigrationSQL).WillReturnRows(readinessRowsFixture(2, []any{101, true}))
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "migration_pair_rejected" || r.FailureStage != "migration_after" || r.TransactionReleased == nil || !*r.TransactionReleased {
			t.Fatal("snapshot_hid_live_version_change")
		}
		assertReadinessPrivate(t, r)
	})
	t.Run("release_beyond_original_deadline", func(t *testing.T) {
		db, m := readinessMock(t)
		readinessPrefix(m)
		readinessCatalogExpectations(m)
		readinessOperationExpectations(m)
		readinessFinishExpectations(m)
		r := probeReadiness(context.Background(), db, fixtureConnection, fixtureBinding())
		r.deadline = time.Now().Add(-time.Nanosecond)
		m.ExpectClose()
		r = releaseReadinessDatabase(db, r)
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_deadline_exceeded" {
			t.Fatal("release_reset_whole_deadline")
		}
		assertReadinessPrivate(t, r)
	})
	t.Run("page_uses_parent_deadline", func(t *testing.T) {
		db, m := readinessMock(t)
		c, e := db.Conn(context.Background())
		if e != nil {
			t.Fatal("fixture_connection_failed")
		}
		m.ExpectQuery(readinessPageSQL(false, false)).WithArgs("0").WillDelayFor(50 * time.Millisecond).WillReturnRows(readinessRowsFixture(2))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		r := baseReadinessReceipt(fixtureBinding())
		p, _, eof, err := readinessPartitionPages(ctx, c, &r, false, sql.NullString{})
		if err == nil || eof || p != (scalarPartition{}) || ctx.Err() != context.DeadlineExceeded {
			t.Fatal("page_reset_whole_deadline")
		}
		if c.Close() != nil {
			t.Fatal("fixture_release_failed")
		}
		m.ExpectClose()
		if db.Close() != nil {
			t.Fatal("fixture_pool_release_failed")
		}
	})
}
