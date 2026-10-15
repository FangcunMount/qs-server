package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit-only trusted-executor fixture; production never manufactures this proof.
func testCaptureReceipt(m material, hash string) receipt {
	return receipt{Action: "capture", ToolSource: m.ToolSource, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, Complete: true, ReadOnly: true, MaterialHash: hash, SQLConnectionClosed: true, SQLPoolClosed: true, MongoClosed: true, Error: "none"}
}

func testCaptureEvidence(t *testing.T, m material, hash string) []string {
	t.Helper()
	var stdout bytes.Buffer
	exit := deliverReceipt(testCaptureReceipt(m, hash), &stdout)
	return testCaptureExecution(t, stdout.Bytes(), exit)
}

func testCaptureExecution(t *testing.T, stdout []byte, exit int) []string {
	t.Helper()
	dir := privateTestDir(t)
	rp, ep := filepath.Join(dir, "capture-stdout.json"), filepath.Join(dir, "capture-execution.json")
	if writeExclusive(rp, stdout) != nil {
		t.Fatal("private stdout fixture unavailable")
	}
	raw, _ := json.Marshal(captureExecutionEvidence{Format: 1, Kind: "m4_a_capture_execution", ReceiptFile: rp, ReceiptHash: fingerprint(stdout), Termination: "exited", ExitCode: &exit})
	if writeExclusive(ep, raw) != nil {
		t.Fatal("private execution fixture unavailable")
	}
	return []string{"--capture-evidence-file=" + ep, "--capture-evidence-sha256=" + fingerprint(raw)}
}

func TestCaptureEvidenceRequiresActualSuccessfulExitAndExactReceipt(t *testing.T) {
	m := testMaterial()
	hash := strings.Repeat("d", 64)
	base := testCaptureReceipt(m, hash)
	for _, name := range []string{"valid", "exit_nonzero", "exit_missing", "exit_null", "exit_string", "exit_alias", "exit_alias_override", "exit_duplicate", "termination_unknown", "kind_unknown", "format_null", "unknown_field", "relative_receipt", "receipt_null", "receipt_oversize", "envelope_oversize", "receipt_hash", "receipt_symlink", "receipt_mode", "action", "incomplete", "complete_alias", "complete_alias_override", "complete_duplicate", "error", "material_hash", "tool_source", "A_source", "resources", "resource_count", "not_readonly", "sql_connection", "sql_pool", "mongo", "release_unknown", "transaction_unknown", "budget", "production_claim", "receipt_unknown_field", "missing_budget", "null_budget", "missing_production_claim", "null_production_claim", "missing_transaction_flag", "null_transaction_flag", "null_empty_release"} {
		t.Run(name, func(t *testing.T) {
			r := base
			switch name {
			case "action":
				r.Action = "inspect"
			case "incomplete":
				r.Complete = false
			case "error":
				r.Error = "capture_parent_changed"
			case "material_hash":
				r.MaterialHash = strings.Repeat("f", 64)
			case "tool_source":
				r.ToolSource = strings.Repeat("2", 40)
			case "A_source":
				r.ASource = strings.Repeat("3", 40)
			case "resources":
				r.AResources = strings.Repeat("a", 64)
			case "resource_count":
				r.AResourceFiles--
			case "not_readonly":
				r.ReadOnly = false
			case "sql_connection":
				r.SQLConnectionClosed = false
			case "sql_pool":
				r.SQLPoolClosed = false
			case "mongo":
				r.MongoClosed = false
			case "release_unknown":
				r.ReleaseError = "owned_connection_release_unproven"
			case "transaction_unknown":
				r.TransactionReleaseUnproven = true
			case "budget":
				r.BudgetExceeded = true
			case "production_claim":
				r.ProductionFreezeCertified = true
			}
			stdout, _ := json.Marshal(r)
			if name == "complete_alias_override" {
				stdout = bytes.Replace(stdout, []byte(`"complete":true`), []byte(`"complete":false`), 1)
				stdout = append(stdout[:len(stdout)-1], []byte(`,"Complete":true}`)...)
			}
			if name == "complete_duplicate" {
				stdout = bytes.Replace(stdout, []byte(`"complete":true`), []byte(`"complete":false`), 1)
				stdout = append(stdout[:len(stdout)-1], []byte(`,"complete":true}`)...)
			}
			if name == "complete_alias" || strings.HasPrefix(name, "missing_") || strings.HasPrefix(name, "null_") {
				var fields map[string]any
				if json.Unmarshal(stdout, &fields) != nil {
					t.Fatal("typed receipt fixture unavailable")
				}
				switch name {
				case "complete_alias":
					fields["Complete"] = fields["complete"]
					delete(fields, "complete")
				case "missing_budget":
					delete(fields, "total_budget_exceeded")
				case "null_budget":
					fields["total_budget_exceeded"] = nil
				case "missing_production_claim":
					delete(fields, "production_freeze_certified")
				case "null_production_claim":
					fields["production_freeze_certified"] = nil
				case "missing_transaction_flag":
					delete(fields, "transaction_release_unproven")
				case "null_transaction_flag":
					fields["transaction_release_unproven"] = nil
				case "null_empty_release":
					fields["release_error_category"] = nil
				}
				stdout, _ = json.Marshal(fields)
			}
			if name == "receipt_unknown_field" {
				stdout = append(stdout[:len(stdout)-1], []byte(`,"unexpected":true}`)...)
			}
			if name == "receipt_oversize" {
				stdout = bytes.Repeat([]byte(" "), 65537)
			}
			dir := privateTestDir(t)
			rp, ep := filepath.Join(dir, "stdout.json"), filepath.Join(dir, "execution.json")
			if writeExclusive(rp, stdout) != nil {
				t.Fatal("stdout fixture unavailable")
			}
			e := map[string]any{"format_version": 1, "kind": "m4_a_capture_execution", "receipt_file": rp, "receipt_sha256": fingerprint(stdout), "termination": "exited", "exit_code": 0}
			switch name {
			case "exit_nonzero":
				e["exit_code"] = 1
			case "exit_missing":
				delete(e, "exit_code")
			case "exit_null":
				e["exit_code"] = nil
			case "exit_string":
				e["exit_code"] = "0"
			case "exit_alias":
				delete(e, "exit_code")
				e["Exit_Code"] = 0
			case "exit_alias_override":
				e["exit_code"] = 1
			case "exit_duplicate":
				e["exit_code"] = 1
			case "termination_unknown":
				e["termination"] = "unknown"
			case "kind_unknown":
				e["kind"] = "m4_d_capture_execution"
			case "format_null":
				e["format_version"] = nil
			case "unknown_field":
				e["unexpected"] = true
			case "relative_receipt":
				e["receipt_file"] = "stdout.json"
			case "receipt_null":
				e["receipt_file"] = nil
			case "receipt_hash":
				e["receipt_sha256"] = strings.Repeat("f", 64)
			case "receipt_symlink":
				link := filepath.Join(dir, "alias.json")
				if os.Symlink(rp, link) != nil {
					t.Fatal("symlink fixture unavailable")
				}
				e["receipt_file"] = link
			case "receipt_mode":
				if os.Chmod(rp, 0644) != nil {
					t.Fatal("mode fixture unavailable")
				}
			}
			raw, _ := json.Marshal(e)
			if name == "exit_alias_override" {
				raw = append(raw[:len(raw)-1], []byte(`,"Exit_Code":0}`)...)
			}
			if name == "exit_duplicate" {
				raw = append(raw[:len(raw)-1], []byte(`,"exit_code":0}`)...)
			}
			if name == "envelope_oversize" {
				raw = bytes.Repeat([]byte(" "), 32769)
			}
			if writeExclusive(ep, raw) != nil {
				t.Fatal("execution fixture unavailable")
			}
			err := readCaptureEvidence(m, hash, testSource, ep, fingerprint(raw))
			if (err == nil) != (name == "valid") {
				t.Fatal("capture execution accepted invalid evidence or rejected success", name, err)
			}
		})
	}
}

func TestCaptureEvidenceShapeAllowsMemberOrderAndWhitespace(t *testing.T) {
	r := testCaptureReceipt(testMaterial(), strings.Repeat("d", 64))
	raw, e := json.MarshalIndent(r, "", "  ")
	var got receipt
	if e != nil || decodeCaptureEvidence(append(raw, '\n'), &got) != nil || got != r {
		t.Fatal("actual typed receipt whitespace changed proof semantics")
	}
	var execution captureExecutionEvidence
	if decodeCaptureEvidence([]byte(`{ "exit_code":0, "termination":"exited", "receipt_sha256":"hash", "receipt_file":"/private/file", "kind":"m4_a_capture_execution", "format_version":1 }`), &execution) != nil {
		t.Fatal("literal member order incorrectly required")
	}
}

func TestFailedCaptureMaterialBytesCannotAuthorizeConsumers(t *testing.T) {
	for _, failure := range []string{"parent_after_write", "budget_after_write"} {
		t.Run(failure, func(t *testing.T) {
			m := testMaterial()
			m.CapturePath = filepath.Join(privateTestDir(t), "retained-failed-material.json")
			raw, _ := json.Marshal(m)
			parent, e := captureParent(m.CapturePath)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = parent.close() })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r := testCaptureReceipt(m, "")
			r.Complete = false
			err := finishCapture(ctx, &r, m.CapturePath, &parent, raw, func(p *captureParentIdentity, path string, value []byte) error {
				if e := writeCaptureExclusive(p, path, value); e != nil {
					return e
				}
				if failure == "parent_after_write" {
					return os.Chmod(filepath.Dir(path), 0777)
				}
				cancel()
				return nil
			})
			if err == nil || r.Complete || r.MaterialHash != "" {
				t.Fatal("late capture failure incorrectly certified")
			}
			r.Error = err.Error()
			if os.Chmod(filepath.Dir(m.CapturePath), 0700) != nil {
				t.Fatal("restore own fixture directory mode")
			}
			hash := fingerprint(raw)
			if got, e := readPrivate(m.CapturePath, hash, maxMaterialBytes); e != nil || !bytes.Equal(got, raw) {
				t.Fatal("diagnostic bytes lost", e)
			}
			var stdout bytes.Buffer
			exit := deliverReceipt(r, &stdout)
			if exit != 1 {
				t.Fatal("late failure received success exit")
			}
			proof := testCaptureExecution(t, stdout.Bytes(), exit)
			ap := testApproval(t, m, hash)
			apr, _ := json.Marshal(ap)
			path := filepath.Join(filepath.Dir(m.CapturePath), "independent-approval.json")
			if writeExclusive(path, apr) != nil {
				t.Fatal("approval fixture unavailable")
			}
			for _, action := range []string{"verify-target", "restore", "verify-restored"} {
				for _, withEvidence := range []bool{false, true} {
					args := []string{"--action=" + action, "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + m.CapturePath, "--material-sha256=" + hash}
					if action != "verify-target" {
						args = append(args, "--approval-file="+path, "--approval-sha256="+fingerprint(apr))
					}
					if withEvidence {
						args = append(args, proof...)
					}
					got := run(t.Context(), args, func(string) string { t.Fatal("failed capture reached ENV"); return "" }, testSource, nil)
					if got.Complete || got.SQLConnectionClosed || got.SQLPoolClosed || got.MongoClosed || got.Error == "none" {
						t.Fatal("failed capture material authorized consumer", action, got)
					}
				}
			}
			boundary, claim, e := recoveryPaths(m, hash, ap.Operation)
			journal, je := recoveryJournalPath(m, hash)
			if e != nil || je != nil {
				t.Fatal("fixed recovery paths unavailable")
			}
			for _, state := range []string{boundary, claim, journal} {
				if _, e := os.Stat(state); !os.IsNotExist(e) {
					t.Fatal("failed capture proof consumed approval or initialized state")
				}
			}
			if got, e := readPrivate(m.CapturePath, hash, maxMaterialBytes); e != nil || !bytes.Equal(got, raw) {
				t.Fatal("rejection rewrote diagnostic bytes", e)
			}
		})
	}
}

func TestSuccessfulCaptureEvidenceEntersOriginalConnectionValidation(t *testing.T) {
	for _, action := range []string{"verify-target", "restore", "verify-restored"} {
		t.Run(action, func(t *testing.T) {
			m := testMaterial()
			ap, mp, mh, path, ah := boundTestRestoreInputs(t, &m, privateTestDir(t))
			args := []string{"--action=" + action, "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + mp, "--material-sha256=" + mh}
			args = append(args, testCaptureEvidence(t, m, mh)...)
			if action != "verify-target" {
				args = append(args, "--approval-file="+path, "--approval-sha256="+ah)
			}
			if action == "verify-restored" && consumeApproval(m, ap, ah) != nil {
				t.Fatal("existing approval state unavailable")
			}
			called := false
			r := run(context.Background(), args, func(string) string { called = true; return "invalid-selected-input" }, testSource, nil)
			if !called || r.Complete || r.Error != "selected_connection_input_rejected" {
				t.Fatal("complete evidence failed to reach existing connection boundary", r)
			}
		})
	}
}

func TestCaptureEvidenceArgumentPairAndActionScope(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs"}
	proof := []string{"--capture-evidence-file=/private/capture-execution.json", "--capture-evidence-sha256=" + strings.Repeat("a", 64)}
	for _, action := range []string{"inspect", "capture"} {
		args := append(append([]string(nil), base...), "--action="+action)
		if action == "capture" {
			args = append(args, "--capture-file=/private/material.json")
		}
		for _, fields := range [][]string{proof[:1], proof[1:], proof, {"--capture-evidence-file="}, {"--capture-evidence-sha256="}} {
			if _, e := parseArgs(append(append([]string(nil), args...), fields...)); e == nil {
				t.Fatal("non-consumer accepted capture evidence", action)
			}
		}
	}
	for _, fields := range [][]string{nil, proof[:1], proof[1:]} {
		args := append(append([]string(nil), base...), "--action=verify-target", "--material-file=/private/material.json", "--material-sha256="+strings.Repeat("b", 64))
		if _, e := parseArgs(append(args, fields...)); e == nil {
			t.Fatal("consumer accepted incomplete evidence pair")
		}
	}
}

func TestCaptureReceiptHashBindsOriginalMaterialFacts(t *testing.T) {
	for _, changed := range []string{"row_count", "identity", "namespace", "EOF"} {
		t.Run(changed, func(t *testing.T) {
			m := testMaterial()
			m.CapturePath = filepath.Join(privateTestDir(t), "material.json")
			original, _ := json.Marshal(m)
			proof := testCaptureEvidence(t, m, fingerprint(original))
			switch changed {
			case "row_count":
				m.Tables[0].Rows = m.Tables[0].Rows[:1]
			case "identity":
				m.SQLIdentity = strings.Repeat("f", 64)
			case "namespace":
				m.SQLName = "qs_different"
			case "EOF":
				m.Tables[0].EOF = false
			}
			raw, _ := json.Marshal(m)
			if writeExclusive(m.CapturePath, raw) != nil {
				t.Fatal("material fixture unavailable")
			}
			args := append([]string{"--action=verify-target", "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + m.CapturePath, "--material-sha256=" + fingerprint(raw)}, proof...)
			r := run(t.Context(), args, func(string) string { t.Fatal("changed original facts reached ENV"); return "" }, testSource, nil)
			if r.Complete || r.Error == "none" {
				t.Fatal("first receipt hash failed to anchor full original material")
			}
		})
	}
}
