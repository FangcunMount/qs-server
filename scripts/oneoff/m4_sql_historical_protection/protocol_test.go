package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSource = "1111111111111111111111111111111111111111"

func privateTestDir(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func testMaterial() material {
	now := time.Now().UTC()
	m := material{Format: 1, Kind: "m4_a_sql_null_object_protection", ToolSource: testSource, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, CapturedAt: now.Format(time.RFC3339Nano), SQLName: "qs_fixture", MongoName: "qs_fixture", SQLIdentity: strings.Repeat("a", 64), MongoIdentity: strings.Repeat("b", 64), MongoUUID: strings.Repeat("c", 64), MongoHead: []byte{1}, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, LedgerDDL: "CREATE TABLE schema_migrations", Ledger: []ledgerRow{{100, false}}}
	for i, o := range objects {
		check, e := originalCheckDefinition(i)
		if e != nil {
			panic(e)
		}
		ddl := "CREATE TABLE `" + o.table + "` (\n  `id` bigint unsigned NOT NULL,\n  `" + o.column + "` json DEFAULT NULL,\n  PRIMARY KEY (`id`),\n  " + check + "\n) ENGINE=InnoDB AUTO_INCREMENT=3"
		m.Tables[i] = tableProof{Table: o.table, OriginalDDL: ddl, RetainedDDL: normalizeDDL(ddl, i), Columns: []column{{"id", "bigint unsigned"}}, Rows: []rowProof{{0, strings.Repeat("d", 64)}, {^uint64(0), strings.Repeat("e", 64)}}, Pages: 2, EOF: true, SQLNullOnly: true}
	}
	return m
}

func TestOriginalDefinitionDriftRefusedBeforeRestoration(t *testing.T) {
	for i := range objects {
		original := testMaterial().Tables[i].OriginalDDL
		if e := validateOriginalObjectDefinition(original, i); e != nil {
			t.Fatal(e)
		}
		for _, changed := range []string{
			strings.Replace(original, "<= 524288", "<= 524287", 1),
			strings.Replace(original, "DEFAULT NULL", "INVISIBLE DEFAULT NULL", 1),
			strings.Replace(original, "  PRIMARY KEY", "  `unexpected_after` int DEFAULT NULL,\n  PRIMARY KEY", 1),
			strings.Replace(original, "is null", "is not null", 1),
			strings.Replace(original, "\"maxItems\":128", "\"maxItems\":127", 1),
		} {
			if changed == original || validateOriginalObjectDefinition(changed, i) == nil {
				t.Fatal("unrestorable definition accepted")
			}
			m := testMaterial()
			m.Tables[i].OriginalDDL = changed
			m.Tables[i].RetainedDDL = normalizeDDL(changed, i)
			if validateMaterial(m, testSource, m.SQLName, m.MongoName) == nil {
				t.Fatal("bound material may not authorize an unknown original definition")
			}
		}
	}
}
func testApproval(t *testing.T, m material, hash string) approval {
	t.Helper()
	now := time.Now().UTC()
	return approval{Format: 1, Kind: "m4_a_pre_start_sql_object_restore", Operation: "123-1", ToolSource: testSource, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, MaterialHash: hash, FreezeHash: strings.Repeat("f", 64), ResponsibilityHash: strings.Repeat("a", 64), SQLIdentity: m.SQLIdentity, MongoIdentity: m.MongoIdentity, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(privateTestDir(t), "first-runtime.marker"), WindowStarted: now.Add(-time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Format(time.RFC3339Nano), Deadline: now.Add(time.Minute).Format(time.RFC3339Nano), Expires: now.Add(2 * time.Minute).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
}
func TestMaterialFixedScopeAndCompleteRange(t *testing.T) {
	m := testMaterial()
	if e := validateMaterial(m, testSource, m.SQLName, m.MongoName); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct {
		name string
		f    func(*material)
	}{
		{"wrong_tool", func(m *material) { m.ToolSource = aSource }}, {"wrong_A", func(m *material) { m.ASource = testSource }}, {"main_resources", func(m *material) { m.AResources = strings.Repeat("0", 64) }}, {"dirty", func(m *material) { m.Ledger[0].Dirty = true }}, {"future", func(m *material) { m.Ledger[0].Version = 102 }}, {"more_ledger_rows", func(m *material) { m.Ledger = append(m.Ledger, ledgerRow{99, false}) }}, {"zone", func(m *material) { m.SessionZone = "SYSTEM" }}, {"eof_missing", func(m *material) { m.Tables[0].EOF = false }}, {"non_null", func(m *material) { m.Tables[0].SQLNullOnly = false }}, {"different_table", func(m *material) { m.Tables[1].Table = "testee" }}, {"unordered_ids", func(m *material) { m.Tables[0].Rows[1].ID = 0 }}, {"body_column", func(m *material) {
			m.Tables[0].Columns = append(m.Tables[0].Columns, column{objects[0].column, "json"})
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			v := testMaterial()
			change.f(&v)
			if validateMaterial(v, testSource, v.SQLName, v.MongoName) == nil {
				t.Fatal("unbound/incomplete material accepted")
			}
		})
	}
}
func TestRestoreIndependentApprovalAndFirstRuntimeBoundary(t *testing.T) {
	m := testMaterial()
	h := strings.Repeat("d", 64)
	a := testApproval(t, m, h)
	if e := validateApproval(a, m, h, testSource, time.Now()); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct {
		name string
		f    func(*approval)
	}{
		{"self_hash", func(a *approval) { a.MaterialHash = strings.Repeat("c", 64) }}, {"freeze_missing", func(a *approval) { a.RuntimeFrozen = false }}, {"already_started", func(a *approval) { a.RuntimeNotStarted = false }}, {"resource_changed", func(a *approval) { a.AResources = strings.Repeat("e", 64) }}, {"expired", func(a *approval) { a.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano) }}, {"over_hour", func(a *approval) { a.Deadline = time.Now().Add(2 * time.Hour).Format(time.RFC3339Nano) }}, {"not_T0_capture", func(a *approval) { a.WindowStarted = time.Now().Add(time.Second).Format(time.RFC3339Nano) }}, {"no_retention", func(a *approval) { a.RetentionDays = 3 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			v := testApproval(t, m, h)
			change.f(&v)
			if validateApproval(v, m, h, testSource, time.Now()) == nil {
				t.Fatal("invalid approval accepted")
			}
		})
	}
	if e := os.WriteFile(a.Marker, []byte("issued before runtime command"), 0600); e != nil {
		t.Fatal(e)
	}
	if validateApproval(a, m, h, testSource, time.Now()) == nil {
		t.Fatal("runtime marker accepted")
	}
	if _, e := os.Stat(a.Marker); e != nil {
		t.Fatal("tool removed marker")
	}
}
func TestFirstCaptureExclusiveExternalHashAndApprovalSingleUse(t *testing.T) {
	p := filepath.Join(privateTestDir(t), "material.json")
	raw, _ := json.Marshal(testMaterial())
	if e := writeExclusive(p, raw); e != nil {
		t.Fatal(e)
	}
	h := fingerprint(raw)
	if _, e := readPrivate(p, h, maxMaterialBytes); e != nil {
		t.Fatal(e)
	}
	if writeExclusive(p, []byte("replacement")) == nil {
		t.Fatal("first capture overwritten")
	}
	if _, e := readPrivate(p, strings.Repeat("e", 64), maxMaterialBytes); e == nil {
		t.Fatal("restore input self-authorized")
	}
	a := testApproval(t, testMaterial(), h)
	approvalPath := filepath.Join(privateTestDir(t), "approval.json")
	araw, _ := json.Marshal(a)
	if e := writeExclusive(approvalPath, araw); e != nil {
		t.Fatal(e)
	}
	if e := consumeApproval(approvalPath, a, fingerprint(araw)); e != nil {
		t.Fatal(e)
	}
	if consumeApproval(approvalPath, a, fingerprint(araw)) == nil {
		t.Fatal("approval reused")
	}
	if e := os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(p, h, maxMaterialBytes); e == nil {
		t.Fatal("public material accepted")
	}
	if e := os.Chmod(p, 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(filepath.Dir(p), "link")
	if e := os.Link(p, link); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(p, h, maxMaterialBytes); e == nil {
		t.Fatal("hardlinked material accepted")
	}
}
func TestNoArbitraryRestorationResourceOrDDLNormalization(t *testing.T) {
	got, e := restorationStatements()
	if e != nil {
		t.Fatal(e)
	}
	for i, s := range got {
		if !strings.HasPrefix(s, "ALTER TABLE `"+objects[i].table+"`") || strings.Contains(s, "DROP TABLE") {
			t.Fatal("unbounded restoration")
		}
	}
	raw, e := os.ReadFile("../../../internal/pkg/migration/migrations/mysql/000099_business_historical_references.up.sql")
	if e != nil || fingerprint(raw) != originalDDLHash || string(raw) != originalDDL {
		t.Fatal("original migration bytes or immutable copy differ", e)
	}
	m := testMaterial()
	ddl := m.Tables[0].OriginalDDL
	if normalizeDDL(ddl, 0) == normalizeDDL(strings.Replace(ddl, "bigint unsigned", "bigint", 1), 0) {
		t.Fatal("unrelated structure change erased")
	}
	if normalizeDDL(ddl, 0) != normalizeDDL(strings.Replace(ddl, "AUTO_INCREMENT=3", "AUTO_INCREMENT=4", 1), 0) {
		t.Fatal("dynamic auto increment not narrowly handled")
	}
	sql := retainedHashSQL([]column{{"id", "bigint unsigned"}, {"payload_json", "longtext"}})
	if !strings.Contains(sql, "IS NULL THEN 'N;' ELSE CONCAT('V',OCTET_LENGTH") || !strings.Contains(sql, "HEX(CAST") || strings.Contains(sql, "CONCAT_WS") {
		t.Fatal("ambiguous retained encoding")
	}
}
func TestDefaultReadOnlyAndCredentialsNeverFlags(t *testing.T) {
	a, e := parseArgs([]string{"--mysql-database=qs", "--mongo-database=qs"})
	if e != nil || a.action != "inspect" {
		t.Fatal(a, e)
	}
	for _, args := range [][]string{{"--mysql-database=qs", "--mongo-database=qs", "--password=secret"}, {"--mysql-database=qs", "--mongo-database=qs", "--action=force"}, {"--mysql-database=qs", "--mongo-database=qs", "--timeout=21m"}, {"--mysql-database=qs", "--mongo-database=qs", "--action=restore"}, {"--mysql-database=qs", "--mongo-database=qs", "--capture-file=/tmp/file"}} {
		if _, e := parseArgs(args); e == nil {
			t.Fatal("unsafe arguments accepted")
		}
	}
	r := run(t.Context(), []string{"--mysql-database=qs", "--mongo-database=qs"}, func(string) string { return "mysql://secret" }, "")
	if r.Complete || strings.Contains(r.Error, "secret") || r.ProductionFreezeCertified {
		t.Fatal(r)
	}
}

func TestMongoDirtyMustExistWithStrictTypeAndNoDuplicate(t *testing.T) {
	for _, item := range []struct {
		name string
		doc  bson.D
		want bool
	}{
		{"clean", bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}, true},
		{"int32_clean", bson.D{{Key: "version", Value: int32(39)}, {Key: "dirty", Value: false}}, true},
		{"missing_dirty", bson.D{{Key: "version", Value: int64(39)}}, false},
		{"null_dirty", bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: nil}}, false},
		{"string_dirty", bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: "false"}}, false},
		{"dirty", bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: true}}, false},
		{"float_version", bson.D{{Key: "version", Value: float64(39)}, {Key: "dirty", Value: false}}, false},
		{"duplicate", bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: true}, {Key: "dirty", Value: false}}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			raw, e := bson.Marshal(item.doc)
			if e != nil {
				t.Fatal(e)
			}
			if cleanMongo39(raw) != item.want {
				t.Fatal("unproved BSON clean state")
			}
		})
	}
}
func TestTransactionalTableScopeMustBeCompleteAndInnoDB(t *testing.T) {
	for _, engine := range []string{"InnoDB", "MyISAM", "MEMORY"} {
		t.Run(engine, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				mock.ExpectClose()
				if e := db.Close(); e != nil {
					t.Error(e)
				}
			}()
			rows := sqlmock.NewRows([]string{"TABLE_NAME", "TABLE_TYPE", "ENGINE"}).AddRow("assessment", "BASE TABLE", "InnoDB").AddRow("evaluation_outcome", "BASE TABLE", "InnoDB").AddRow("schema_migrations", "BASE TABLE", engine)
			mock.ExpectQuery("SELECT TABLE_NAME,TABLE_TYPE,ENGINE").WillReturnRows(rows)
			e = requireTransactionalTables(t.Context(), db)
			if (e == nil) != (engine == "InnoDB") {
				t.Fatal("transactionality gate", e)
			}
			if e = mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestCaptureFsyncCrossingDeadlineNeverCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := receipt{}
	e := finishCapture(ctx, &r, "private", []byte("captured"), func(string, []byte) error { cancel(); return nil })
	if e == nil || r.Complete {
		t.Fatal("late file release was marked complete")
	}
}

type rollbackFailure struct{ err error }

func (r rollbackFailure) Rollback() error { return r.err }

type failedReceiptWriter struct{}

func (failedReceiptWriter) Write([]byte) (int, error) {
	return 0, errors.New("private output detail must not be printed")
}
func TestRollbackFailureKeepsOperationCategoryWithoutFalseRelease(t *testing.T) {
	for _, failure := range []error{nil, sql.ErrTxDone, errors.New("private transport failure")} {
		operation := errors.New("ledger_commit_outcome_unknown")
		e := operation
		recordRollback(&e, rollbackFailure{failure})
		if e.Error() != operation.Error() {
			t.Fatal("operation category replaced or raw provider error exposed", e)
		}
		want := failure != nil && !errors.Is(failure, sql.ErrTxDone)
		if hasTransactionReleaseFailure(e) != want {
			t.Fatal("transaction release was overcertified")
		}
	}
}
func TestReceiptOutputFailureNeverExitsSuccessfully(t *testing.T) {
	args := []string{"--mysql-database=qs", "--mongo-database=qs"}
	if deliverReceipt(receipt{Complete: true}, failedReceiptWriter{}) == 0 {
		t.Fatal("receipt write error exited0")
	}
	if deliverReceipt(receipt{Complete: true}, io.Discard) != 0 {
		t.Fatal("delivered positive receipt did not exit0")
	}
	if serve(t.Context(), args, func(string) string { return "" }, testSource, io.Discard) == 0 {
		t.Fatal("failed action exited0")
	}
}

func TestPoolCloseMustProveNoBorrowedConnectionsRemain(t *testing.T) {
	db, mock, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	c, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	mock.ExpectClose()
	if closeSQLPool(db) {
		t.Fatal("Close(nil) with a still borrowed connection was signed released")
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	s := db.Stats()
	if s.OpenConnections != 0 || s.InUse != 0 {
		t.Fatal("test owner failed to release connection")
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
