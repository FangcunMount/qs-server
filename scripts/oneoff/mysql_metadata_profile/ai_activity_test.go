package main

import (
	"context"
	"github.com/go-sql-driver/mysql"
	"strings"
	"testing"
)

func TestAIActivityExact43PreservesBusyCountsAndCanonicalIdentity(t *testing.T) {
	r := mockAIActivity(t, aiHead43, false)
	if !r.Complete || r.Measurements == nil || r.QueryCount != 24 || r.FreezeVerified || r.ObservationsAtomic || !r.ReadOnly || !r.Measurements.TargetPresent || r.Measurements.TargetRows == nil || *r.Measurements.TargetRows != 7 || r.Measurements.TargetDDLSHA256 == nil || !r.Measurements.GlobalMetadataGrantsVerified || r.Measurements.Counts["ai_modifying_transactions"] == 0 {
		t.Fatal(r)
	}
}
func TestAIActivity44AbsenceIsNotAnEmptyTable(t *testing.T) {
	r := mockAIActivity(t, aiHead44, false)
	if !r.Complete || r.QueryCount != 22 || r.Measurements.TargetPresent || r.Measurements.TargetRows != nil || r.Measurements.TargetDDLSHA256 != nil {
		t.Fatal(r)
	}
}
func TestAIActivityPermissionFailureDiscardsAllPartialObservations(t *testing.T) {
	r := mockAIActivity(t, aiHead43, true)
	if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_permission_denied" || r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1142 || r.ConnectionReleased == nil || !*r.ConnectionReleased {
		t.Fatal(r)
	}
}
func TestAIActivityWrongTargetStopsBeforeGrants(t *testing.T) {
	db, m := readinessMock(t)
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	b := fixtureBinding()
	b.Expected = strings.Repeat("f", 64)
	r := probeAIActivity(context.Background(), db, fixtureConnection, b)
	if r.Complete || r.QueryCount != 1 || r.ErrorCategory != "target_hash_mismatch" || r.Measurements != nil {
		t.Fatal(r)
	}
}
func mockAIActivity(t *testing.T, head string, denied bool) aiActivityReceipt {
	t.Helper()
	db, m := readinessMock(t)
	m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(aiLedgerSQL).WillReturnRows(readinessRowsFixture(1, []any{head}))
	present := 0
	if head == aiHead43 {
		present = 1
	}
	m.ExpectQuery(aiTargetPresenceSQL).WillReturnRows(readinessRowsFixture(1, []any{present}))
	if present == 1 {
		m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM messaging_outbox").WillReturnRows(readinessRowsFixture(1, []any{7}))
		m.ExpectQuery("SHOW CREATE TABLE messaging_outbox").WillReturnRows(readinessRowsFixture(2, []any{"messaging_outbox", "CREATE TABLE messaging_outbox(id INT)"}))
	}
	m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM performance_schema.setup_instruments WHERE NAME='wait/lock/metadata/sql/mdl' AND ENABLED='YES'").WillReturnRows(readinessRowsFixture(1, []any{1}))
	m.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=?").WithArgs(uint64(10)).WillReturnRows(readinessRowsFixture(1, []any{20}))
	for i, q := range aiActivityQueries {
		id := uint64(10)
		if strings.Contains(q.name, "metadata_locks_") {
			id = 20
		}
		if denied && q.name == "ai_metadata_locks_pending" {
			m.ExpectQuery(q.query).WithArgs(id).WillReturnError(&mysql.MySQLError{Number: 1142, Message: fixtureSecret})
			break
		}
		m.ExpectQuery(q.query).WithArgs(id).WillReturnRows(readinessRowsFixture(1, []any{i + 1}))
	}
	if !denied {
		m.ExpectQuery("SELECT CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6))*1000000 AS UNSIGNED)").WillReturnRows(readinessRowsFixture(1, []any{1793}))
		m.ExpectQuery(readinessIdentitySQL).WillReturnRows(readinessIdentities(10, "NONE"))
	}
	return probeAIActivity(context.Background(), db, fixtureConnection, fixtureBinding())
}
