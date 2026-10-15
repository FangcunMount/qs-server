package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func TestLegacyMaterialCannotGuessInitialDatabaseBoundary(t *testing.T) {
	m := originalMaterial()
	m.Format = 2
	m.CapturePath = filepath.Join(privateFixture(t), "legacy.json")
	raw, e := json.Marshal(m)
	if e != nil || writeExclusive(m.CapturePath, raw) != nil {
		t.Fatal("owned legacy material unavailable")
	}
	hash := fingerprint(raw)
	args := append(cliArgs("verify-target"), "--material-file="+m.CapturePath, "--material-sha256="+hash)
	args = append(args, unitCaptureEvidenceArgs(t, m, hash)...)
	readEnvironment := false
	r := run(context.Background(), args, func(string) string { readEnvironment = true; return "" }, testTool)
	if readEnvironment || r.Complete || r.Error != "original_material_rejected" {
		t.Fatal("legacy material reached ENV without a frozen database generation", r, readEnvironment)
	}
}

func TestFrozenBoundaryRequiresExplicitLiteralGenerationAndHash(t *testing.T) {
	hash := initialRuntimeBoundary(testBinding()).Fingerprint
	for _, raw := range []string{"null", "{}", `{"state_sha256":"` + hash + `"}`, `{"generation":null,"state_sha256":"` + hash + `"}`, `{"generation":0,"state_sha256":null}`, `{"Generation":0,"state_sha256":"` + hash + `"}`, `{"generation":0,"generation":0,"state_sha256":"` + hash + `"}`} {
		var state runtimeBoundaryState
		if strictDecode([]byte(raw), &state) == nil {
			t.Fatal("missing/null/aliased generation guessed initial", raw)
		}
	}
	var state runtimeBoundaryState
	if strictDecode([]byte(`{"generation":0,"state_sha256":"`+hash+`"}`), &state) != nil || state != initialRuntimeBoundary(testBinding()) {
		t.Fatal("explicit qualified initial generation rejected")
	}
}

func TestVerifyTargetRejectsRuntimeBoundaryDrift(t *testing.T) {
	original := originalMaterial()
	target := cloneMaterial(original)
	target.Ledger = []ledgerRow{{103, false}}
	target.Tables = [5]tableProof{}
	if verifyTarget(original, target) != nil {
		t.Fatal("unchanged target fixture rejected")
	}
	record, raw := boundaryRecordFixture(t, original)
	target.RuntimeBoundary = runtimeBoundaryState{Generation: record.Generation, Fingerprint: fingerprint(raw)}
	if verifyTarget(original, target) == nil {
		t.Fatal("stale material obtained a complete target proof after another runtime intent")
	}
}

func expectInitialRuntimeBoundary(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnError(&mysql.MySQLError{Number: 1146, Message: "private missing-table text"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='m4_d_runtime_boundary'")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

func runtimeDatabaseFixture(t *testing.T, conn *sql.Conn, b binding) database {
	t.Helper()
	key, e := runtimeGateKey(b)
	if e != nil {
		t.Fatal(e)
	}
	return database{conn: conn, binding: b, gate: &runtimeGate{conn: conn, key: key, acquired: true}}
}

func expectCapturedRuntimeBoundary(mock sqlmock.Sqlmock, b binding) {
	key, _ := runtimeGateKey(b)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	expectInitialRuntimeBoundary(mock)
}

func boundaryRecordFixture(t *testing.T, m material) (runtimeBoundaryRecord, []byte) {
	t.Helper()
	v := runtimeIntent{Mode: "cancel102"}
	a := arguments{materialHash: fingerprint([]byte("original material")), runtimeIntentHash: fingerprint([]byte("approved intent"))}
	record, raw, e := nextRuntimeBoundary(m, v, a, testTool, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	return record, raw
}

func expectStoredRuntimeBoundary(mock sqlmock.Sqlmock, record runtimeBoundaryRecord, raw []byte) {
	hash, _ := hex.DecodeString(fingerprint(raw))
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(runtimeBoundaryTable, runtimeBoundaryDDL))
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundarySelect)).WillReturnRows(sqlmock.NewRows([]string{"singleton", "generation", "state_sha256", "state_json"}).AddRow(1, record.Generation, hash, raw))
}

func TestRuntimeBoundaryMissingRequiresTypedServerAbsenceAndEmptyCatalog(t *testing.T) {
	for _, name := range []string{"initial", "permission", "wire", "hidden_object", "catalog_unknown"} {
		t.Run(name, func(t *testing.T) {
			db, mock := dbFixture(t)
			errorValue := error(&mysql.MySQLError{Number: 1146, Message: "private server text"})
			if name == "permission" {
				errorValue = &mysql.MySQLError{Number: 1142, Message: "private permission text"}
			}
			if name == "wire" {
				errorValue = errors.New("private missing-table text is not a typed absence")
			}
			mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnError(errorValue)
			if name != "permission" && name != "wire" {
				q := mock.ExpectQuery("SELECT COUNT")
				if name == "catalog_unknown" {
					q.WillReturnError(errors.New("private catalog failure"))
				} else {
					count := 0
					if name == "hidden_object" {
						count = 1
					}
					q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
				}
			}
			got, e := readRuntimeBoundary(t.Context(), db, testBinding())
			if (e == nil) != (name == "initial") || e == nil && got.State != initialRuntimeBoundary(testBinding()) {
				t.Fatal("absence/permission promoted to an initial generation", name, got, e)
			}
		})
	}
}

func TestRuntimeBoundarySchemaAndSingleCanonicalRowAreClosed(t *testing.T) {
	for _, name := range []string{"current", "foreign_schema", "empty", "extra_row", "wrong_identity", "wrong_hash", "zero_generation", "unknown_json", "noncanonical_json", "rows_release"} {
		t.Run(name, func(t *testing.T) {
			db, mock := dbFixture(t)
			record, raw := boundaryRecordFixture(t, originalMaterial())
			ddl := runtimeBoundaryDDL
			if name == "foreign_schema" {
				ddl += " COMMENT='foreign'"
			}
			mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(runtimeBoundaryTable, ddl))
			if name != "foreign_schema" {
				if name == "wrong_identity" {
					record.SQLIdentity = fingerprint([]byte("other server"))
					raw, _ = json.Marshal(record)
				}
				if name == "zero_generation" {
					record.Generation = 0
					raw, _ = json.Marshal(record)
				}
				if name == "unknown_json" {
					raw = append(append([]byte(nil), raw[:len(raw)-1]...), []byte(",\"unknown\":true}")...)
				}
				if name == "noncanonical_json" {
					raw = append([]byte(" "), raw...)
				}
				hash, _ := hex.DecodeString(fingerprint(raw))
				if name == "wrong_hash" {
					hash[0] ^= 1
				}
				rows := sqlmock.NewRows([]string{"singleton", "generation", "state_sha256", "state_json"})
				if name != "empty" {
					rows.AddRow(1, record.Generation, hash, raw)
				}
				if name == "extra_row" {
					rows.AddRow(1, record.Generation, hash, raw)
				}
				if name == "rows_release" {
					rows.RowError(0, errors.New("private row release failure"))
				}
				mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundarySelect)).WillReturnRows(rows)
			}
			got, e := readRuntimeBoundary(t.Context(), db, testBinding())
			if (e == nil) != (name == "current") || e == nil && (got.Record == nil || *got.Record != record || got.State.Generation != 1 || got.State.Fingerprint != fingerprint(raw)) {
				t.Fatal("unknown schema/row became usable", name, got, e)
			}
		})
	}
}

func TestAnotherLegalCaptureCannotRestoreAfterDatabaseAdvance(t *testing.T) {
	first, _, _, _, _ := recoveryInputs(t)
	second, approval, hash, approvalPath, approvalHash := recoveryInputs(t)
	if first.CapturePath == second.CapturePath || first.Binding != second.Binding || first.RuntimeBoundary != second.RuntimeBoundary {
		t.Fatal("two independent legal captures not reproduced")
	}
	record, raw := boundaryRecordFixture(t, first)
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	d := runtimeDatabaseFixture(t, conn, second.Binding)
	key, _ := runtimeGateKey(second.Binding)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	expectStoredRuntimeBoundary(mock, record, raw)
	consumed := false
	e = d.restore(t.Context(), second, approval, second.CapturePath, hash, approvalPath, approvalHash, "", "", testTool, &consumed)
	if e == nil || e.Error() != "runtime_database_boundary_changed" || consumed {
		t.Fatal("different capture directory crossed the database boundary", e, consumed)
	}
	_, claim, e := recoveryPaths(second, hash, approval.Operation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		t.Fatal("stale capture consumed an approval before rejecting")
	}
}

func TestFreshClean102CaptureCanFreezeNextGenerationWithoutReset(t *testing.T) {
	first := originalMaterial()
	record, raw := boundaryRecordFixture(t, first)
	next := cloneMaterial(first)
	next.RuntimeBoundary = runtimeBoundaryState{Generation: record.Generation, Fingerprint: fingerprint(raw)}
	if validateMaterial(next, testTool, next.Binding) != nil {
		t.Fatal("new capture cannot freeze a qualified current generation")
	}
	advanced, _ := boundaryRecordFixture(t, next)
	if advanced.Generation != 2 || advanced.Previous != next.RuntimeBoundary.Fingerprint || next.RuntimeBoundary == first.RuntimeBoundary {
		t.Fatal("new maintenance attempt reset or failed to advance the boundary")
	}
	next.RuntimeBoundary.Generation = math.MaxUint64
	if _, _, e := nextRuntimeBoundary(next, runtimeIntent{Mode: "cancel102"}, arguments{materialHash: fingerprint([]byte("m")), runtimeIntentHash: fingerprint([]byte("i"))}, testTool, time.Now()); e == nil {
		t.Fatal("overflow reset the durable generation")
	}
}

func TestInitialBoundaryCreateUnknownDoesNotGrantRuntime(t *testing.T) {
	m := originalMaterial()
	db, mock := dbFixture(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	d := runtimeDatabaseFixture(t, conn, m.Binding)
	expectCapturedRuntimeBoundary(mock, m.Binding)
	mock.ExpectExec(regexp.QuoteMeta(runtimeBoundaryCreate)).WithArgs(1, uint64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(errors.New("private create acknowledgment lost"))
	r := receipt{}
	e = d.advanceRuntimeBoundary(t.Context(), m, runtimeIntent{Mode: "cancel102"}, arguments{materialHash: fingerprint([]byte("m")), runtimeIntentHash: fingerprint([]byte("i"))}, testTool, &r)
	if e == nil || e.Error() != "runtime_database_boundary_create_outcome_unknown" || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten {
		t.Fatal("unknown initialization granted runtime or was retried", e, r)
	}
}

func TestRuntimeBoundaryUpdateUsesOriginalGenerationHashAndBytesCAS(t *testing.T) {
	for _, outcome := range []string{"committed", "compare_conflict", "commit_unknown"} {
		t.Run(outcome, func(t *testing.T) {
			m := originalMaterial()
			before, oldRaw := boundaryRecordFixture(t, m)
			current := runtimeBoundaryObservation{State: runtimeBoundaryState{Generation: before.Generation, Fingerprint: fingerprint(oldRaw)}, Record: &before, Raw: oldRaw}
			m.RuntimeBoundary = current.State
			next, raw := boundaryRecordFixture(t, m)
			hash, _ := hex.DecodeString(fingerprint(raw))
			oldHash, _ := hex.DecodeString(current.State.Fingerprint)
			db, mock := dbFixture(t)
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			d := runtimeDatabaseFixture(t, conn, m.Binding)
			mock.ExpectBegin()
			expectStoredRuntimeBoundary(mock, before, oldRaw)
			count := int64(1)
			if outcome == "compare_conflict" {
				count = 0
			}
			mock.ExpectExec(regexp.QuoteMeta(runtimeBoundaryUpdate)).WithArgs(next.Generation, hash, raw, current.State.Generation, oldHash, oldRaw).WillReturnResult(sqlmock.NewResult(0, count))
			if outcome == "compare_conflict" {
				mock.ExpectRollback()
			} else {
				expectStoredRuntimeBoundary(mock, next, raw)
				if outcome == "commit_unknown" {
					mock.ExpectCommit().WillReturnError(errors.New("private commit acknowledgment lost"))
				} else {
					mock.ExpectCommit()
				}
			}
			r := receipt{}
			e = d.updateRuntimeBoundary(t.Context(), current, next, raw, hash, m.Binding, &r)
			if (e == nil) != (outcome == "committed") || !r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundaryAdvanced || r.MarkerWritten || m.RuntimeBoundary != current.State {
				t.Fatal("CAS/unknown commit changed original capture or granted startup", outcome, e, r)
			}
			if outcome == "compare_conflict" && e.Error() != "runtime_database_boundary_compare_and_set_rejected" || outcome == "commit_unknown" && e.Error() != "runtime_database_boundary_commit_outcome_unknown" {
				t.Fatal("CAS/commit classification lost", outcome, e)
			}
		})
	}
}
