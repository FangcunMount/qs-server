package main

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecoveryGateIsSameDatabaseDomainNotApprovalOrMaterial(t *testing.T) {
	b := fixtureBinding()
	first, e := recoveryGateKey(b)
	if e != nil || len(first) != 64 {
		t.Fatal("bounded server lock name unavailable")
	}
	b.MongoUUID = fingerprint([]byte("different ledger"))
	second, _ := recoveryGateKey(b)
	if second != first {
		t.Fatal("non-SQL input split same-database mutation gate")
	}
	b.SQLName = "another_owned_database"
	other, _ := recoveryGateKey(b)
	if other == first {
		t.Fatal("different database borrowed the same local C gate")
	}
}

func TestRecoveryGateRequiresActualOwnershipAndReleaseACK(t *testing.T) {
	db, mock := recoveryGateSQLMock(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	key, _ := recoveryGateKey(fixtureBinding())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?,0)")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	g, e := acquireRecoveryGate(t.Context(), conn, fixtureBinding())
	if e != nil || !g.acquired || !g.releaseUnproven {
		t.Fatal("missing actual lease evidence", e)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	if g.require(t.Context()) != nil {
		t.Fatal("actual same-session owner rejected")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	if g.release() != nil || !g.released || g.releaseUnproven || g.require(context.Background()) == nil {
		t.Fatal("released lease authorized a later write")
	}
}

func TestRecoveryGateBusyNullAndLostAcquireNeverAuthorizeMutation(t *testing.T) {
	for _, kind := range []string{"busy", "null", "wire_error"} {
		t.Run(kind, func(t *testing.T) {
			db, mock := recoveryGateSQLMock(t)
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			key, _ := recoveryGateKey(fixtureBinding())
			q := mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?,0)")).WithArgs(key)
			switch kind {
			case "busy":
				q.WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))
			case "null":
				q.WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
			default:
				q.WillReturnError(errors.New("private driver text"))
			}
			g, e := acquireRecoveryGate(t.Context(), conn, fixtureBinding())
			if e == nil || g.acquired || g.require(t.Context()) == nil || g.releaseUnproven != (kind == "wire_error") {
				t.Fatal("failed/unknown acquisition authorized write")
			}
		})
	}
}

func TestRecoveryGateReleaseUnknownSurvivesLocalConnectionClose(t *testing.T) {
	db, mock := recoveryGateSQLMock(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	key, _ := recoveryGateKey(fixtureBinding())
	g := &recoveryGate{conn: conn, key: key, acquired: true, releaseRequired: true, releaseUnproven: true}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(key).WillReturnError(errors.New("private release response lost"))
	if g.release() == nil || conn.Close() != nil || g.released || !g.releaseUnproven {
		t.Fatal("local closure was promoted to server lease release")
	}
}

func recoveryGateSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := mock.ExpectationsWereMet(); e != nil {
			t.Error(e)
		}
		_ = db.Close()
	})
	return db, mock
}

func TestRecoveryGateLosesOwnershipBeforeAnyMutation(t *testing.T) {
	for _, kind := range []string{"other_session", "null", "wire_error", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			db, mock := recoveryGateSQLMock(t)
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			key, _ := recoveryGateKey(fixtureBinding())
			g := &recoveryGate{conn: conn, key: key, acquired: true}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if kind == "canceled" {
				cancel()
			} else {
				q := mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key)
				switch kind {
				case "other_session":
					q.WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(false))
				case "null":
					q.WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(nil))
				default:
					q.WillReturnError(errors.New("private ownership response lost"))
				}
			}
			if g.require(ctx) == nil {
				t.Fatal("lost ownership authorized further work")
			}
		})
	}
}
