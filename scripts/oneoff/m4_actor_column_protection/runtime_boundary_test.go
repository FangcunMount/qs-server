package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func boundaryRecordFixture(t *testing.T, m material) (runtimeBoundaryRecord, []byte) {
	t.Helper()
	v := runtimeBoundaryRecord{Format: 1, Kind: "m4_c_runtime_boundary", SQLIdentity: m.Binding.SQLIdentity, SQLName: m.Binding.SQLName, Generation: m.RuntimeBoundary.Generation + 1, Previous: m.RuntimeBoundary.Fingerprint, ToolSource: fixtureSource, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, MaterialHash: fingerprint([]byte("original material")), IntentHash: fingerprint([]byte("independent intent")), Mode: "cancel101", SQLVersion: 101, Created: "2026-10-15T00:00:01Z"}
	v.MarkerRaw, _ = json.Marshal(markerForRecord(v))
	v.MarkerHash = fingerprint(v.MarkerRaw)
	raw, _ := json.Marshal(v)
	if e := validateRuntimeBoundaryRecord(v, raw, m.Binding); e != nil {
		t.Fatal(e)
	}
	return v, raw
}

func expectInitialRuntimeBoundary(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnError(&mysql.MySQLError{Number: 1146})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='m4_c_runtime_boundary'")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

func expectStoredRuntimeBoundary(mock sqlmock.Sqlmock, record runtimeBoundaryRecord, raw []byte) {
	hash, _ := hex.DecodeString(fingerprint(raw))
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(runtimeBoundaryTable, runtimeBoundaryDDL))
	mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundarySelect)).WillReturnRows(sqlmock.NewRows([]string{"singleton", "generation", "state_sha256", "state_json"}).AddRow(1, record.Generation, hash, raw))
}

func TestLegacyMaterialCannotGuessInitialRuntimeBoundary(t *testing.T) {
	m, _, _, _, _ := recoveryInputs(t)
	m.Format = 1
	// The independently supplied original SHA cannot upgrade a legacy format.
	path := m.CapturePath + ".legacy"
	m.CapturePath = path
	raw, _ := json.Marshal(m)
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	args := actorConsumerArgs(m, fingerprint(raw), "verify-target", "", "")
	args = append(args, "--capture-evidence-file="+path+".evidence", "--capture-evidence-sha256="+fingerprint([]byte("unused")))
	r := run(context.Background(), args, func(string) string { t.Fatal("legacy material reached ENV"); return "" }, fixtureSource)
	if r.Complete || r.Error != "original_material_rejected" {
		t.Fatal("old material guessed a generation", r)
	}
}

func TestRuntimeBoundaryRequiresLiteralGenerationAndHash(t *testing.T) {
	hash := initialRuntimeBoundary(fixtureBinding()).Fingerprint
	for _, raw := range []string{"null", "{}", `{"state_sha256":"` + hash + `"}`, `{"generation":null,"state_sha256":"` + hash + `"}`, `{"generation":0,"state_sha256":null}`, `{"Generation":0,"state_sha256":"` + hash + `"}`, `{"generation":0,"generation":0,"state_sha256":"` + hash + `"}`} {
		var s runtimeBoundaryState
		if strictDecode([]byte(raw), &s) == nil {
			t.Fatal("missing/null/alias guessed initial state", raw)
		}
	}
	var s runtimeBoundaryState
	if strictDecode([]byte(`{"generation":0,"state_sha256":"`+hash+`"}`), &s) != nil || !validRuntimeBoundaryState(s, fixtureBinding()) {
		t.Fatal("explicit initial state rejected")
	}
	m := fixtureMaterial(t)
	m.RuntimeBoundary = runtimeBoundaryState{}
	if validateMaterial(m, fixtureSource, m.Binding) == nil {
		t.Fatal("format2 missing generation accepted")
	}
}

func TestRuntimeBoundaryAbsenceRequiresTypedMissingAndEmptyCatalog(t *testing.T) {
	for _, name := range []string{"initial", "permission", "wire", "hidden", "catalog_unknown"} {
		t.Run(name, func(t *testing.T) {
			db, mock := recoveryGateSQLMock(t)
			err := error(&mysql.MySQLError{Number: 1146})
			switch name {
			case "permission":
				err = &mysql.MySQLError{Number: 1142}
			case "wire":
				err = errors.New("missing table is not a typed server response")
			}
			mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnError(err)
			if name != "permission" && name != "wire" {
				q := mock.ExpectQuery("SELECT COUNT")
				if name == "catalog_unknown" {
					q.WillReturnError(errors.New("private failure"))
				} else {
					count := 0
					if name == "hidden" {
						count = 1
					}
					q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
				}
			}
			got, e := readRuntimeBoundary(t.Context(), db, fixtureBinding())
			if (e == nil) != (name == "initial") || e == nil && got.State != initialRuntimeBoundary(fixtureBinding()) {
				t.Fatal("unknown state promoted to initial", got, e)
			}
		})
	}
}

func TestRuntimeBoundaryRejectsForeignSchemaAndNoncanonicalRows(t *testing.T) {
	for _, name := range []string{"current", "foreign_schema", "view", "empty", "extra", "identity", "hash", "generation", "unknown_json", "noncanonical", "release", "marker_bytes", "marker_hash"} {
		t.Run(name, func(t *testing.T) {
			db, mock := recoveryGateSQLMock(t)
			record, raw := boundaryRecordFixture(t, fixtureMaterial(t))
			ddl := runtimeBoundaryDDL
			if name == "foreign_schema" {
				ddl += " COMMENT='foreign'"
			}
			if name == "view" {
				ddl = "CREATE VIEW `m4_c_runtime_boundary` AS SELECT 1"
			}
			mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundaryShow)).WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(runtimeBoundaryTable, ddl))
			if name != "foreign_schema" && name != "view" {
				switch name {
				case "identity":
					record.SQLName = "another_database"
				case "generation":
					record.Generation = 0
				case "marker_bytes":
					record.MarkerRaw = append(record.MarkerRaw, ' ')
				case "marker_hash":
					record.MarkerHash = fingerprint([]byte("other"))
				}
				raw, _ = json.Marshal(record)
				if name == "unknown_json" {
					raw = append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"unknown":true}`)...)
				}
				if name == "noncanonical" {
					raw = append([]byte(" "), raw...)
				}
				hash, _ := hex.DecodeString(fingerprint(raw))
				if name == "hash" {
					hash[0] ^= 1
				}
				rows := sqlmock.NewRows([]string{"singleton", "generation", "state_sha256", "state_json"})
				if name != "empty" {
					rows.AddRow(1, record.Generation, hash, raw)
				}
				if name == "extra" {
					rows.AddRow(1, record.Generation, hash, raw)
				}
				if name == "release" {
					rows.RowError(0, errors.New("private release failure"))
				}
				mock.ExpectQuery(regexp.QuoteMeta(runtimeBoundarySelect)).WillReturnRows(rows)
			}
			got, e := readRuntimeBoundary(t.Context(), db, fixtureBinding())
			if (e == nil) != (name == "current") || e == nil && (got.Record == nil || got.State.Fingerprint != fingerprint(raw)) {
				t.Fatal("unknown schema/record became authoritative", got, e)
			}
		})
	}
}

func TestAllFactsComparisonsRejectAnotherRuntimeGeneration(t *testing.T) {
	original := fixtureMaterial(t)
	restored := cloneMaterial(t, original)
	restored.RuntimeBoundary.Generation++
	if verifyRestored(original, restored) == nil {
		t.Fatal("restored old capture crossed runtime generation")
	}
	target := cloneMaterial(t, original)
	target.Ledger = []ledgerRow{{102, false}}
	for i := range target.Tables {
		target.Tables[i].DDL = target.Tables[i].RetainedDDL
		for n := range target.Tables[i].Rows {
			target.Tables[i].Rows[n].Value = nil
		}
	}
	if verifyTarget(original, target) != nil {
		t.Fatal("unchanged target rejected")
	}
	target.RuntimeBoundary.Generation++
	if verifyTarget(original, target) == nil {
		t.Fatal("target crossed runtime generation")
	}
	state := currentRecovery(t, original, columnAbsent, columnAbsent, true)
	state.Material.RuntimeBoundary.Generation++
	if validateRecoveryState(original, state, false) == nil {
		t.Fatal("partial restore crossed runtime generation")
	}
}

func TestAnotherCaptureCannotConsumeAfterDatabaseRuntimeAdvance(t *testing.T) {
	first, _, _, _, _ := recoveryInputs(t)
	second, a, hash, path, approvalHash := recoveryInputs(t)
	if first.CapturePath == second.CapturePath || first.Binding != second.Binding || first.RuntimeBoundary != second.RuntimeBoundary {
		t.Fatal("two legal captures did not share a database generation")
	}
	db, mock := recoveryGateSQLMock(t)
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	key, _ := recoveryGateKey(second.Binding)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT IS_USED_LOCK(?)=CONNECTION_ID()")).WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"owned"}).AddRow(1))
	record, raw := boundaryRecordFixture(t, first)
	expectStoredRuntimeBoundary(mock, record, raw)
	d := database{conn: conn, binding: second.Binding, gate: &recoveryGate{conn: conn, key: key, acquired: true}}
	consumed := false
	e = d.restore(t.Context(), second, a, second.CapturePath, hash, path, approvalHash, "", "", fixtureSource, &consumed)
	if e == nil || e.Error() != "runtime_database_boundary_changed" || consumed {
		t.Fatal("different capture crossed the persistent runtime boundary", e, consumed)
	}
	_, claim, e := recoveryPaths(second, hash, a.Operation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		t.Fatal("stale capture consumed an operation")
	}
}
