package main

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRuntimeGateIsSameDatabaseDomainNotApprovalOrMaterial(t *testing.T) {
	b := testBinding()
	first, e := runtimeGateKey(b)
	if e != nil || len(first) != 64 {
		t.Fatal("bounded server lock name unavailable")
	}
	b.MongoUUID = fingerprint([]byte("different ledger"))
	second, _ := runtimeGateKey(b)
	if second != first {
		t.Fatal("non-SQL input split same-database mutation gate")
	}
	b.SQLName = "another_owned_database"
	other, _ := runtimeGateKey(b)
	if other == first {
		t.Fatal("different database borrowed the same local D gate")
	}
}

func TestRuntimeGateRequiresActualOwnershipAndReleaseACK(t *testing.T) {
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	key, _ := runtimeGateKey(testBinding())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?,0)")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	g, e := acquireRuntimeGate(t.Context(), conn, testBinding())
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

func TestRuntimeGateBusyNullAndLostAcquireNeverAuthorizeMutation(t *testing.T) {
	for _, kind := range []string{"busy", "null", "wire_error"} {
		t.Run(kind, func(t *testing.T) {
			db, mock := dbFixture(t)
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			key, _ := runtimeGateKey(testBinding())
			q := mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?,0)")).WithArgs(key)
			switch kind {
			case "busy":
				q.WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))
			case "null":
				q.WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
			default:
				q.WillReturnError(errors.New("private driver text"))
			}
			g, e := acquireRuntimeGate(t.Context(), conn, testBinding())
			if e == nil || g.acquired || g.require(t.Context()) == nil || g.releaseUnproven != (kind == "wire_error") {
				t.Fatal("failed/unknown acquisition authorized write")
			}
		})
	}
}

func TestRuntimeGateReleaseUnknownSurvivesLocalConnectionClose(t *testing.T) {
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	key, _ := runtimeGateKey(testBinding())
	g := &runtimeGate{conn: conn, key: key, acquired: true, releaseRequired: true, releaseUnproven: true}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).WithArgs(key).WillReturnError(errors.New("private release response lost"))
	if g.release() == nil || conn.Close() != nil || g.released || !g.releaseUnproven {
		t.Fatal("local closure was promoted to server lease release")
	}
}
