package main

import (
	"context"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestM5ActivityFailureDiscardsPartialCounts(t *testing.T) {
	db, m := readinessMock(t)
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ version,dirty FROM schema_migrations LIMIT 2").WillReturnRows(readinessRowsFixture(2, []any{111, true}))
	m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.setup_instruments WHERE NAME='wait/lock/metadata/sql/mdl' AND ENABLED='YES'").WillReturnRows(readinessRowsFixture(1, []any{1}))
	m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=?").WithArgs(uint64(10)).WillReturnRows(readinessRowsFixture(1, []any{20}))
	for i, q := range m5ActivityQueries {
		id := uint64(10)
		if strings.HasPrefix(q.name, "qs_metadata_locks_") {
			id = 20
		}
		if q.name == "qs_metadata_locks_pending" {
			m.ExpectQuery(q.query).WithArgs(id).WillReturnError(&mysql.MySQLError{Number: 1142, Message: fixtureSecret})
			break
		}
		m.ExpectQuery(q.query).WithArgs(id).WillReturnRows(readinessRowsFixture(1, []any{i + 1}))
	}
	r := probeM5Activity(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.Complete || r.Measurements != nil || r.FreezeVerified || !r.ReadOnly || r.ErrorCategory != "query_permission_denied" || r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1142 || r.ConnectionReleased == nil || !*r.ConnectionReleased {
		t.Fatal("unknown activity became zero/qualified", r)
	}
}

func TestM5ActivityIsSeparateAndRefusesWrongTargetBeforeGrantRead(t *testing.T) {
	db, m := readinessMock(t)
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	b := fixtureBinding()
	b.Expected = strings.Repeat("f", 64)
	r := probeM5Activity(context.Background(), db, fixtureConnection, b)
	if r.Complete || r.QueryCount != 1 || r.ErrorCategory != "target_hash_mismatch" || r.Measurements != nil {
		t.Fatal(r)
	}
}
