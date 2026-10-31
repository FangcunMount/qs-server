package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUnavailableInventoryNeverBecomesEmptyOrDeletionApproval(t *testing.T) {
	for _, failure := range []string{"missing", "query", "structure", "state"} {
		t.Run(failure, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"name", "version", "uuid"}).AddRow("fixture", "8.0.36", "fixture-uuid"))
			mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*MIN\\(version\\)").WillReturnRows(sqlmock.NewRows([]string{"count", "version", "dirty"}).AddRow(1, 110, false))
			q := mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*information_schema.tables").WithArgs("rm_outbox")
			switch failure {
			case "missing":
				q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			case "query":
				q.WillReturnError(context.DeadlineExceeded)
			default:
				q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				structure := mock.ExpectQuery("SHOW CREATE TABLE")
				if failure == "structure" {
					structure.WillReturnError(errors.New("missing table"))
				} else {
					structure.WillReturnRows(sqlmock.NewRows([]string{"table", "ddl"}).AddRow("rm_outbox", "CREATE TABLE fixture"))
					mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*COALESCE").WillReturnError(context.DeadlineExceeded)
				}
			}
			mock.ExpectRollback()
			r, err := collect(context.Background(), db, "qs.mysql", "fixture:3307")
			if err == nil || r.Complete || r.DropAuthorized || r.BusinessVerified || r.ExportVerified || r.WritersExcluded || r.Mutation || !r.ReadOnly || len(r.Tables) != 1 || r.Tables[0].Rows != nil {
				t.Fatal("unavailable evidence was treated as empty or authorized", r, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnsupportedOwnersRefuseBeforeDatabaseUse(t *testing.T) {
	for _, owner := range []string{"", "qs.mongo", "ai.mysql", "qs.mysql; DROP TABLE rm_outbox"} {
		if _, err := collect(context.Background(), nil, owner, ""); err == nil {
			t.Fatal(owner)
		}
	}
}

func TestFailedTransactionReleaseInvalidatesCompletedMeasurements(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"name", "version", "uuid"}).AddRow("fixture", "8.0.36", "fixture-uuid"))
	mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*MIN\\(version\\)").WillReturnRows(sqlmock.NewRows([]string{"count", "version", "dirty"}).AddRow(1, 110, false))
	for _, name := range []string{"rm_outbox", "ai_messaging_outbox"} {
		mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*information_schema.tables").WithArgs(name).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SHOW CREATE TABLE").WillReturnRows(sqlmock.NewRows([]string{"name", "ddl"}).AddRow(name, "CREATE TABLE fixture"))
		mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*COALESCE").WillReturnRows(sqlmock.NewRows([]string{"total", "closed", "other", "null"}).AddRow(0, 0, 0, 0))
	}
	mock.ExpectRollback().WillReturnError(errors.New("connection lost during release"))
	r, err := collect(context.Background(), db, "qs.mysql", "fixture")
	if err == nil || r.Complete || r.TransactionReleased || r.DropAuthorized {
		t.Fatal(r, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAmbiguousMissingOrDirtySchemaHeadCannotBecomeComplete(t *testing.T) {
	for _, state := range []struct {
		rows    int
		version any
		dirty   bool
	}{{0, nil, false}, {2, 110, false}, {1, 110, true}} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT DATABASE").WillReturnRows(sqlmock.NewRows([]string{"name", "version", "uuid"}).AddRow("fixture", "8.0.36", "fixture-uuid"))
		mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME.*MIN\\(version\\)").WillReturnRows(sqlmock.NewRows([]string{"count", "version", "dirty"}).AddRow(state.rows, state.version, state.dirty))
		mock.ExpectRollback()
		r, err := collect(context.Background(), db, "qs.mysql", "fixture")
		if err == nil || r.Complete || !r.TransactionReleased || len(r.Tables) != 0 || r.DropAuthorized {
			t.Fatal(r, err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
}
