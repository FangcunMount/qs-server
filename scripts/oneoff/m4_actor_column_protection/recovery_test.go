package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func currentRecovery(t *testing.T, m material, tags, roles columnState, dirty bool) recoveryState {
	t.Helper()
	out := recoveryState{Material: cloneMaterial(t, m), Columns: [2]columnState{tags, roles}}
	out.Material.Ledger = []ledgerRow{{102, dirty}}
	for i, state := range out.Columns {
		table := &out.Material.Tables[i]
		switch state {
		case columnAbsent:
			table.DDL = table.RetainedDDL
			for n := range table.Rows {
				table.Rows[n].Value = nil
			}
		case columnTemporaryNullable:
			table.DDL = strings.Replace(table.DDL, "  `effective_roles` json NOT NULL,\n", "  `effective_roles` json DEFAULT NULL,\n", 1)
			for n := range table.Rows {
				table.Rows[n].Value = &originalValue{Kind: "sql_null"}
			}
		}
	}
	return out
}

func TestRecoveryInitialAndContinuationStatesAreDistinct(t *testing.T) {
	m := fixtureMaterial(t)
	for _, tc := range []struct {
		name                  string
		tags, roles           columnState
		dirty, prior, allowed bool
	}{
		{"dirty_drop0", columnOriginal, columnOriginal, true, false, true},
		{"dirty_drop1", columnAbsent, columnOriginal, true, false, true},
		{"dirty_drop2", columnAbsent, columnAbsent, true, false, true},
		{"clean_drop2", columnAbsent, columnAbsent, false, false, true},
		{"clean_foreign_original", columnOriginal, columnOriginal, false, false, false},
		{"initial_foreign_nullable", columnAbsent, columnTemporaryNullable, true, false, false},
		{"initial_clean_partial", columnAbsent, columnOriginal, false, false, false},
		{"reversed_drop", columnOriginal, columnAbsent, true, true, false},
		{"nullable_before_tags", columnOriginal, columnTemporaryNullable, true, true, false},
		{"resume_nullable", columnAbsent, columnTemporaryNullable, false, true, true},
		{"resume_roles_complete", columnAbsent, columnOriginal, false, true, true},
		{"resume_both_complete", columnOriginal, columnOriginal, false, true, true},
		{"unknown_state", columnOriginal, columnState(9), true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := currentRecovery(t, m, tc.tags, tc.roles, tc.dirty)
			if (validateRecoveryState(m, state, tc.prior) == nil) != tc.allowed {
				t.Fatal("recovery admitted an unproved initial/continuation state")
			}
		})
	}
}

func TestRecoveryPreservesExactRawValuesAndAllFourTables(t *testing.T) {
	m := fixtureMaterial(t)
	for _, original := range []*originalValue{{Kind: "sql_null"}, {Kind: "json", JSON: []byte("null")}, {Kind: "json", JSON: []byte("[]")}, {Kind: "json", JSON: []byte(`{"case":"A","raw":1}`)}} {
		m.Tables[0].Rows[0].Value = original
		s := currentRecovery(t, m, columnOriginal, columnOriginal, false)
		s.Material.Tables[0].Rows[0].Value = &originalValue{Kind: "sql_null"}
		if validateRecoveryState(m, s, true) != nil {
			t.Fatal("legitimate partial original tags rejected")
		}
		s.Material.Tables[0].Rows[0].Value = &originalValue{Kind: "json", JSON: []byte(`{"different":true}`)}
		if validateRecoveryState(m, s, true) == nil {
			t.Fatal("foreign JSON accepted as partial recovery")
		}
	}
	for _, kind := range []string{"new_key", "archive_hash", "archive_ddl", "mongo", "ledger_ddl", "binding", "position", "roles_json", "eof"} {
		t.Run(kind, func(t *testing.T) {
			s := currentRecovery(t, m, columnAbsent, columnTemporaryNullable, false)
			switch kind {
			case "new_key":
				s.Material.Tables[0].Rows = append(s.Material.Tables[0].Rows, rowProof{Key: "1"})
			case "archive_hash":
				s.Material.Tables[3].Rows[0].Retained = strings.Repeat("e", 64)
			case "archive_ddl":
				s.Material.Tables[2].DDL += " COMMENT='foreign'"
			case "mongo":
				s.Material.MongoHead = append(s.Material.MongoHead, 0)
			case "ledger_ddl":
				s.Material.LedgerDDL += " changed"
			case "binding":
				s.Material.Binding.MongoUUID = strings.Repeat("e", 64)
			case "position":
				s.Material.Tables[1].DDL = strings.ReplaceAll(s.Material.Tables[1].DDL, "  `roles` json NOT NULL,\n", "")
			case "roles_json":
				s.Material.Tables[1].Rows[0].Value = &originalValue{Kind: "json", JSON: []byte("null")}
			case "eof":
				s.Material.Tables[2].EOF = false
			}
			if validateRecoveryState(m, s, true) == nil {
				t.Fatal("changed original value or retained fact accepted")
			}
		})
	}
}

func TestOriginalDDLComesOnlyFromPinned001And070(t *testing.T) {
	statements, e := fixedRestorationSQL()
	if e != nil || statements[0] != "ALTER TABLE `testee` ADD COLUMN `tags` json DEFAULT NULL COMMENT '标签（JSON数组）' AFTER `birthday`" || statements[1] != "ALTER TABLE `operators` ADD COLUMN `effective_roles` JSON NULL AFTER `roles`" || statements[2] != "ALTER TABLE `operators` MODIFY COLUMN `effective_roles` JSON NOT NULL" {
		t.Fatal("fixed original source statement mismatch", e)
	}
	m := fixtureMaterial(t)
	for i := 0; i < 2; i++ {
		state, text, e := restoredColumnShape(m.Tables[i].DDL, i)
		if e != nil || state != columnOriginal || text != m.Tables[i].RetainedDDL {
			t.Fatal("original definition was not recognized", e)
		}
		if _, _, e := restoredColumnShape(strings.ReplaceAll(m.Tables[i].DDL, "json", "JSON"), i); e == nil {
			t.Fatal("noncanonical material SQL became executable")
		}
	}
}

type markerResult struct {
	write func()
	n     int64
}

func (r markerResult) LastInsertId() (int64, error) { return 0, nil }
func (r markerResult) RowsAffected() (int64, error) { r.write(); return r.n, nil }

func TestOriginalValueBatchCASCommitAndFailureBoundaries(t *testing.T) {
	for _, kind := range []string{"success", "cas_miss", "write_error", "marker_before_commit", "commit_unknown", "rollback_failed"} {
		t.Run(kind, func(t *testing.T) {
			m, a, hash, _, ah := recoveryInputs(t)
			if consumeApproval(m, a, ah) != nil {
				t.Fatal("owned restore authority unavailable")
			}
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			conn, e := db.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = conn.Close() }()
			query, e := valueBatchSQL(0, 1, m.Tables[0].Columns)
			if e != nil || strings.Contains(query, "AS UNSIGNED") || !strings.Contains(query, "`updated_at`=`updated_at`") || !strings.Contains(query, "`tags` IS NULL") {
				t.Fatal("CAS/identity/timestamp guard missing", e)
			}
			mock.ExpectBegin()
			exec := mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs("0", []byte("[]"), "0", m.Tables[0].Rows[0].Retained)
			switch kind {
			case "write_error", "rollback_failed":
				exec.WillReturnError(errors.New("private driver detail"))
			case "cas_miss":
				exec.WillReturnResult(sqlmock.NewResult(0, 0))
			case "marker_before_commit":
				exec.WillReturnResult(markerResult{write: func() {
					if writeExclusive(a.Marker, []byte("first runtime issued")) != nil {
						t.Error("marker fault unavailable")
					}
				}, n: 1})
			default:
				exec.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			switch kind {
			case "success":
				mock.ExpectCommit()
			case "commit_unknown":
				mock.ExpectCommit().WillReturnError(errors.New("private response lost"))
			case "rollback_failed":
				mock.ExpectRollback().WillReturnError(errors.New("private close detail"))
			default:
				mock.ExpectRollback()
			}
			d := database{conn: conn}
			e = d.commitValueBatch(t.Context(), m, m.Tables[0].Rows, 0, a, hash, fixtureSource)
			if (e == nil) != (kind == "success") || (e != nil && strings.Contains(e.Error(), "private")) {
				t.Fatal("CAS failure became success or leaked driver detail", e)
			}
			var release *transactionReleaseFailure
			if errors.As(e, &release) != (kind == "rollback_failed") {
				t.Fatal("transaction release evidence incorrect", e)
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestRestoreUsesBoundedAbsoluteDeadlineAndOriginalValueEncoding(t *testing.T) {
	m, a, hash, path, ah := recoveryInputs(t)
	if e := rereadApprovedInputs(m, a, m.CapturePath, hash, path, ah, fixtureSource); e != nil {
		t.Fatal(e)
	}
	raw, e := readPrivate(m.CapturePath, hash)
	copyPath := filepath.Join(filepath.Dir(m.CapturePath), "copy.json")
	if e != nil || writeExclusive(copyPath, raw) != nil || rereadApprovedInputs(m, a, copyPath, hash, path, ah, fixtureSource) == nil {
		t.Fatal("material copy authorized restore")
	}
	ctx, c := restoreActionContext(context.Background(), a)
	defer c()
	deadline, bounded := ctx.Deadline()
	if !bounded || time.Until(deadline) <= 0 || time.Until(deadline) > 30*time.Minute {
		t.Fatal("absolute recovery budget lost")
	}
	q, e := valueBatchSQL(1, 2, m.Tables[1].Columns)
	if e != nil || strings.Count(q, "CAST(? AS JSON)") != 2 || strings.Count(q, "AS DECIMAL(20,0)") != 4 || strings.Contains(q, "`roles`=") {
		t.Fatal("batch mutated canonical roles or lost original numeric/JSON encoding", e)
	}
	if !reflect.DeepEqual(m.Tables[0].Rows[0].Value.JSON, []byte("[]")) {
		t.Fatal("raw original value rewritten")
	}
}

func TestNewBoundaryDoesNotAuthorizeForeignPartialState(t *testing.T) {
	m, a, hash, _, ah := recoveryInputs(t)
	hadPriorBoundary, e := readRecoveryBoundary(m, a, hash)
	if e != nil || hadPriorBoundary {
		t.Fatal("fresh capture already had a recovery boundary", e)
	}
	foreign := currentRecovery(t, m, columnAbsent, columnTemporaryNullable, true)
	if consumeApproval(m, a, ah) != nil {
		t.Fatal("owned new boundary unavailable")
	}
	if validateRecoveryState(m, foreign, hadPriorBoundary) == nil {
		t.Fatal("creating a boundary promoted an unproved partial state")
	}
}

func TestLedgerLastRequiresFullyRestoredValuesNotNullAndArchives(t *testing.T) {
	m := fixtureMaterial(t)
	complete := currentRecovery(t, m, columnOriginal, columnOriginal, false)
	if requireFullyRestored(m, complete) != nil {
		t.Fatal("complete original scope rejected")
	}
	for _, kind := range []string{"nullable_roles", "unfilled_tags", "changed_archive", "missing_key"} {
		t.Run(kind, func(t *testing.T) {
			state := currentRecovery(t, m, columnOriginal, columnOriginal, false)
			switch kind {
			case "nullable_roles":
				state.Columns[1] = columnTemporaryNullable
			case "unfilled_tags":
				state.Material.Tables[0].Rows[0].Value = &originalValue{Kind: "sql_null"}
			case "changed_archive":
				state.Material.Tables[3].Rows[0].Retained = strings.Repeat("e", 64)
			case "missing_key":
				state.Material.Tables[0].Rows = nil
			}
			if requireFullyRestored(m, state) == nil {
				t.Fatal("ledger replay admitted an incomplete original scope")
			}
		})
	}
}

func TestCopiedMaterialAndInvalidApprovalRejectedBeforeEnvironment(t *testing.T) {
	for _, kind := range []string{"copy", "expired", "late_material_change", "late_approval_change"} {
		t.Run(kind, func(t *testing.T) {
			m, a, hash, path, ah := recoveryInputs(t)
			args := []string{"--action=restore", "--mysql-database=" + m.Binding.SQLName, "--mongo-database=" + m.Binding.MongoName, "--mysql-identity-sha256=" + m.Binding.SQLIdentity, "--mongo-identity-sha256=" + m.Binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + m.Binding.MongoUUID}
			materialPath := m.CapturePath
			switch kind {
			case "copy":
				raw, e := readPrivate(m.CapturePath, hash)
				materialPath = filepath.Join(filepath.Dir(m.CapturePath), "copied.json")
				if e != nil || writeExclusive(materialPath, raw) != nil {
					t.Fatal("owned material copy unavailable")
				}
			case "expired":
				a.Deadline = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
				raw, _ := json.Marshal(a)
				path = filepath.Join(filepath.Dir(path), "expired.json")
				if writeExclusive(path, raw) != nil {
					t.Fatal("expired approval unavailable")
				}
				ah = fingerprint(raw)
			case "late_material_change":
				if os.WriteFile(materialPath, []byte(`{"changed":true}`), 0600) != nil {
					t.Fatal("owned material fault unavailable")
				}
			case "late_approval_change":
				if os.WriteFile(path, []byte(`{"changed":true}`), 0600) != nil {
					t.Fatal("owned approval fault unavailable")
				}
			}
			args = append(args, "--material-file="+materialPath, "--material-sha256="+hash, "--approval-file="+path, "--approval-sha256="+ah)
			r := run(t.Context(), args, func(string) string { t.Fatal("invalid frozen input accessed connection configuration"); return "" }, fixtureSource)
			if r.Complete || r.ApprovalConsumed || r.ReleaseError != "" || r.TransactionReleaseUnproven || r.SQLPoolClosed || r.MongoClosed {
				t.Fatal("pre-environment refusal fabricated execution or release", r)
			}
		})
	}
}
