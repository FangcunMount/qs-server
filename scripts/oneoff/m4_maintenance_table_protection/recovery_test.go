package main

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func fixtureRecoveryState(m material, n int, dirty bool) recoveryState {
	current := cloneMaterial(m)
	current.Ledger = []ledgerRow{{103, dirty}}
	missing := make([]bool, len(scope))
	for i := 0; i < n; i++ {
		missing[i] = true
		current.Tables[i] = tableProof{}
	}
	return recoveryState{current, missing}
}
func partialTable(t tableProof, count int) tableProof {
	t.Rows = t.Rows[:count]
	t.Pages = (count+pageSize-1)/pageSize + 1
	t.EncodedRowsBytes = 0
	for _, row := range t.Rows {
		raw, _ := json.Marshal(row)
		t.EncodedRowsBytes += len(raw) + 1
	}
	return t
}

func TestRecoveryFirstAdmissionAndInitialSuffixFacts(t *testing.T) {
	m := originalMaterial()
	for n := 0; n <= len(scope); n++ {
		for _, dirty := range []bool{true, false} {
			s := fixtureRecoveryState(m, n, dirty)
			e := validateRecoveryState(m, s, nil)
			if (e == nil) != (dirty || n == len(scope)) {
				t.Fatal("first prefix", n, dirty, e)
			}
		}
	}
	for _, fault := range []string{"partial suffix", "foreign raw", "modern hash", "DDL", "ledger", "Mongo"} {
		s := fixtureRecoveryState(m, 2, true)
		switch fault {
		case "partial suffix":
			s.Material.Tables[3] = partialTable(s.Material.Tables[3], 0)
		case "foreign raw":
			s.Material.Tables[3].Rows[0].Values[4].Bytes = []byte("changed")
		case "modern hash":
			s.Material.Retained[0].Rows[0].Hash = strings.Repeat("b", 64)
		case "DDL":
			s.Material.Tables[3].DDL += " "
		case "ledger":
			s.Material.Ledger[0].Version = 102
		case "Mongo":
			s.Material.MongoHead = []byte{}
		}
		if validateRecoveryState(m, s, nil) == nil {
			t.Fatal("foreign first state accepted", fault)
		}
	}
}

func TestRecoveryContinuationAllowsOnlyOneOriginalRowSubset(t *testing.T) {
	m := originalMaterial()
	prior := &recoveryBoundary{InitialLedger: ledgerRow{103, false}, InitialMissing: []bool{true, true, true, true, true}}
	for n := 0; n <= len(scope); n++ {
		s := fixtureRecoveryState(m, n, false)
		if n < len(scope) {
			s.Material.Tables[n] = partialTable(s.Material.Tables[n], 0)
		}
		if e := validateRecoveryState(m, s, prior); e != nil {
			t.Fatal("legal inverse prefix", n, e)
		}
		if n < len(scope)-1 {
			s.Material.Tables[n+1] = partialTable(s.Material.Tables[n+1], 0)
			if validateRecoveryState(m, s, prior) == nil {
				t.Fatal("multiple partial tables accepted")
			}
		}
	}
	s := fixtureRecoveryState(m, 1, false)
	s.Material.Tables[1].Rows[0].Values[2].Bytes = []byte("2")
	if validateRecoveryState(m, s, prior) == nil {
		t.Fatal("partial foreign row bytes accepted")
	}
	prior.InitialMissing = []bool{true, false, false, false, false}
	s = fixtureRecoveryState(m, 2, false)
	if validateRecoveryState(m, s, prior) == nil {
		t.Fatal("new missing table expanded first scope")
	}
	s = fixtureRecoveryState(m, 0, true)
	if validateRecoveryState(m, s, prior) == nil {
		t.Fatal("initial clean103 flag changed")
	}
}

func TestTrustedRestorationStatementsIgnoreMaterialSQL(t *testing.T) {
	m := originalMaterial()
	for _, collation := range originalCollations {
		for i := range scope {
			m.Tables[i].DDL = originalDDL(i, collation)
		}
		statements, e := restorationStatements(m)
		if e != nil {
			t.Fatal(e)
		}
		for i, query := range statements {
			if !strings.HasPrefix(query, "CREATE TABLE "+scope[i].name+" (") || !strings.Contains(query, "DEFAULT CHARSET=utf8mb4 COLLATE="+collation) || strings.Contains(query, "DROP ") || strings.Count(query, "CREATE TABLE") != 1 {
				t.Fatal("unqualified inverse CREATE", query)
			}
		}
	}
	m.Tables[0].DDL += "; DELETE FROM operators"
	if _, e := restorationStatements(m); e == nil {
		t.Fatal("material SQL entered inverse CREATE")
	}
}

func TestOriginalInsertUsesFixedColumnsAndPreservesRawParameters(t *testing.T) {
	for i, s := range scope {
		q, e := insertBatchSQL(i, 2)
		if e != nil || strings.Count(q, "?") != len(s.columns)*2 || !strings.HasPrefix(q, "INSERT INTO `"+s.name+"`") || strings.Contains(q, "REPLACE") || strings.Contains(q, "UPDATE") {
			t.Fatal(q, e)
		}
	}
	for _, n := range []int{0, insertBatchRows + 1} {
		if _, e := insertBatchSQL(0, n); e == nil {
			t.Fatal("unbounded insert batch")
		}
	}
	m, a, hash, _, ah := recoveryInputs(t)
	if consumeApproval(m, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) != nil {
		t.Fatal("fixture claim")
	}
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	d := runtimeDatabaseFixture(t, conn, m.Binding)
	row := m.Tables[1].Rows[0]
	q, _ := insertBatchSQL(1, 1)
	values := make([]driver.Value, len(row.Values))
	for i, v := range row.Values {
		if !v.SQLNull {
			values[i] = v.Bytes
		}
	}
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectBegin()
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectExec(regexp.QuoteMeta(q)).WithArgs(values...).WillReturnResult(sqlmock.NewResult(0, 1))
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectCommit()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if e := d.insertOriginalBatch(ctx, m, a, hash, testTool, 1, []rowProof{row}); e != nil {
		t.Fatal(e)
	}
}

func TestOriginalInsertRejectsOverBudgetBatchBeforeAnyConnection(t *testing.T) {
	m := originalMaterial()
	rows := []rowProof{originalRow(4, []string{"41"}), originalRow(4, []string{"42"})}
	for i := range rows {
		rows[i].Values[2].Bytes = append(bytes.Repeat([]byte{' '}, 600000), []byte("{}")...)
		rows[i].Hash = rowHash(scope[4].columns, rows[i].Values)
	}
	if e := (database{}).insertOriginalBatch(t.Context(), m, approval{}, "", testTool, 4, rows); e == nil || e.Error() != "original_insert_batch_bytes_rejected" {
		t.Fatal("unbounded INSERT reached connection", e)
	}
}

func TestInsertAcknowledgementMarkerRefusesCommitAndRetainsRollbackFailure(t *testing.T) {
	m, a, hash, _, ah := recoveryInputs(t)
	if consumeApproval(m, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) != nil {
		t.Fatal("fixture claim")
	}
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	d := runtimeDatabaseFixture(t, conn, m.Binding)
	row := m.Tables[4].Rows[0]
	q, _ := insertBatchSQL(4, 1)
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectBegin()
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectExec(regexp.QuoteMeta(q)).WillReturnResult(markerAfterAck{path: a.Marker})
	mock.ExpectRollback().WillReturnError(errors.New("private rollback failure"))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	e = d.insertOriginalBatch(ctx, m, a, hash, testTool, 4, []rowProof{row})
	var release *transactionReleaseFailure
	if e == nil || e.Error() != "first_runtime_marker_unknown_or_started" || !errors.As(e, &release) || strings.Contains(e.Error(), "private") {
		t.Fatal("lock-time marker or transaction release lost", e)
	}
}

func TestInsertCommitUnknownReturnsFailureWithoutRetry(t *testing.T) {
	m, a, hash, _, ah := recoveryInputs(t)
	if consumeApproval(m, a, ah, ledgerRow{103, true}, []bool{true, true, false, false, false}) != nil {
		t.Fatal("fixture claim")
	}
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	row := m.Tables[0].Rows[0]
	query, _ := insertBatchSQL(0, 1)
	d := runtimeDatabaseFixture(t, conn, m.Binding)
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectBegin()
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectCommit().WillReturnError(errors.New("private driver commit outcome"))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	e = d.insertOriginalBatch(ctx, m, a, hash, testTool, 0, []rowProof{row})
	if e == nil || e.Error() != "original_insert_commit_outcome_unknown" || strings.Contains(e.Error(), "private") {
		t.Fatal("unknown COMMIT was retried/promoted or raw error leaked", e)
	}
}

func TestVerifyRestoredRequiresUnchangedWholeScopedProof(t *testing.T) {
	m := originalMaterial()
	if verifyRestored(m, cloneMaterial(m)) != nil {
		t.Fatal("original proof rejected")
	}
	for _, field := range []string{"row", "DDL", "ledger", "retained"} {
		x := cloneMaterial(m)
		switch field {
		case "row":
			x.Tables[2].Rows[0].Values[1].Bytes = []byte("changed")
		case "DDL":
			x.LedgerDDL += " "
		case "ledger":
			x.Ledger[0].Version = 103
		case "retained":
			x.Retained[7].Rows = nil
		}
		if verifyRestored(m, x) == nil {
			t.Fatal("terminal drift accepted", field)
		}
	}
}

func TestVerifyRestoredAuthorizationDoesNotClaimOrResetBoundary(t *testing.T) {
	m, a, mh, ap, ah := recoveryInputs(t)
	if checkRestoredAuthorization(m, a, m.CapturePath, mh, ap, ah, testTool) == nil {
		t.Fatal("unclaimed readonly verification")
	}
	if consumeApproval(m, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) != nil {
		t.Fatal("fixture claim")
	}
	path, _ := recoveryJournalPath(m, mh)
	before, _ := readOwnedPrivate(path, maxRecoveryJournalBytes)
	if e := checkRestoredAuthorization(m, a, m.CapturePath, mh, ap, ah, testTool); e != nil {
		t.Fatal(e)
	}
	after, _ := readOwnedPrivate(path, maxRecoveryJournalBytes)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("readonly terminal proof consumed approval")
	}
	if os.WriteFile(a.Marker, []byte("first-start"), 0600) != nil {
		t.Fatal("marker fixture")
	}
	if checkRestoredAuthorization(m, a, m.CapturePath, mh, ap, ah, testTool) == nil {
		t.Fatal("terminal proof crossed runtime boundary")
	}
}

func TestRestoreAndVerificationRejectUnprovedInputsBeforeEnvironment(t *testing.T) {
	m, a, mh, ap, ah := recoveryInputs(t)
	for _, action := range []string{"restore", "verify-restored"} {
		args := append(cliArgs(action), "--material-file="+m.CapturePath, "--material-sha256="+mh, "--approval-file="+ap, "--approval-sha256="+ah)
		if action == "restore" {
			a.RuntimeFrozen = false
			raw, _ := json.Marshal(a)
			if os.WriteFile(ap, raw, 0600) != nil {
				t.Fatal("approval fixture")
			}
			args[len(args)-1] = "--approval-sha256=" + fingerprint(raw)
		}
		args = append(args, unitCaptureEvidenceArgs(t, m, mh)...)
		r := run(context.Background(), args, func(string) string { t.Fatal("ENV before input rejection"); return "" }, testTool)
		if r.Complete || r.Error != "restore_approval_rejected" || r.ApprovalConsumed || r.SQLPoolClosed || r.MongoClosed {
			t.Fatal("unproved entrypoint reached resources", r)
		}
	}
}

// RowsAffected runs after the successful statement ACK. The new marker must
// still prevent COMMIT; the real database lock-wait matrix is a later task.
type markerAfterAck struct{ path string }

func (markerAfterAck) LastInsertId() (int64, error) { return 0, nil }
func (r markerAfterAck) RowsAffected() (int64, error) {
	if e := writeExclusive(r.path, []byte("first runtime intent after ACK")); e != nil {
		return 0, e
	}
	return 1, nil
}
