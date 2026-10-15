package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Unit tests use an explicitly synthetic qualifying receipt; the native
// executor instead freezes bytes and exit status returned by actual serve.
func qualifyingCaptureReceipt(m material, hash string) receipt {
	r := receipt{Action: "capture", ToolSource: m.ToolSource, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, Complete: true, ReadOnly: true, MaterialHash: hash, MandatoryEOF: true, SQLSnapshot: true, SQLConnectionClosed: true, SQLPoolClosed: true, MongoClosed: true, Error: "none", TableCounts: map[string]int{}}
	for _, table := range m.Tables {
		r.TableCounts[table.Table] = len(table.Rows)
	}
	return r
}

func freezeCaptureExecution(t *testing.T, dir, prefix string, stdout []byte, exit int) (string, string) {
	t.Helper()
	receiptPath := filepath.Join(dir, prefix+"-stdout.json")
	if e := writeExclusive(receiptPath, stdout); e != nil {
		t.Fatal(e)
	}
	execution := captureExecutionEvidence{Format: 1, Kind: "m4_c_capture_execution", Termination: "exited", ExitCode: &exit, ReceiptFile: receiptPath, ReceiptHash: fingerprint(stdout)}
	raw, e := json.Marshal(execution)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, prefix+"-execution.json")
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	return path, fingerprint(raw)
}

func unitCaptureEvidence(t *testing.T, m material, hash, prefix string) []string {
	t.Helper()
	var out bytes.Buffer
	if emitReceipt(qualifyingCaptureReceipt(m, hash), &out) != 0 {
		t.Fatal("qualifying unit capture receipt encoding failed")
	}
	path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), prefix, out.Bytes(), 0)
	return []string{"--capture-evidence-file=" + path, "--capture-evidence-sha256=" + evidenceHash}
}

func actorConsumerArgs(m material, hash, action, approvalPath, approvalHash string) []string {
	args := []string{"--action=" + action, "--mysql-database=" + m.Binding.SQLName, "--mongo-database=" + m.Binding.MongoName, "--mysql-identity-sha256=" + m.Binding.SQLIdentity, "--mongo-identity-sha256=" + m.Binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + m.Binding.MongoUUID, "--material-file=" + m.CapturePath, "--material-sha256=" + hash}
	if action == "restore" {
		args = append(args, "--approval-file="+approvalPath, "--approval-sha256="+approvalHash)
	}
	return args
}

func TestCaptureEvidenceFlagsAreConsumerOnly(t *testing.T) {
	b := fixtureBinding()
	for _, action := range []string{"inspect", "capture"} {
		t.Run(action, func(t *testing.T) {
			args := []string{"--action=" + action, "--mysql-database=" + b.SQLName, "--mongo-database=" + b.MongoName, "--mysql-identity-sha256=" + b.SQLIdentity, "--mongo-identity-sha256=" + b.MongoIdentity, "--mongo-ledger-uuid-sha256=" + b.MongoUUID}
			if action == "capture" {
				args = append(args, "--capture-file=/private/approved/new.json")
			}
			if _, e := parseArgs(args); e != nil {
				t.Fatal("valid non-consumer arguments rejected", e)
			}
			for _, flag := range []string{"--capture-evidence-file=", "--capture-evidence-sha256="} {
				if _, e := parseArgs(append(args, flag)); e == nil || e.Error() != "capture_evidence_action_mismatch" {
					t.Fatal("non-consumer accepted explicit capture evidence flag", flag, e)
				}
			}
		})
	}
}

func TestFailedCaptureRemainderCannotAuthorizeActorConsumers(t *testing.T) {
	for _, failure := range []string{"after_write", "closed_parent", "after_parent_release"} {
		t.Run(failure, func(t *testing.T) {
			m, _, hash, approvalPath, approvalHash := recoveryInputs(t)
			raw, e := readPrivate(m.CapturePath, hash)
			if e != nil || os.Remove(m.CapturePath) != nil {
				t.Fatal("prepare exact original capture bytes")
			}
			parent, e := captureParent(m.CapturePath)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = parent.close() })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			actionContext := ctx
			if failure == "after_parent_release" {
				actionContext = captureReleasedContext{Context: ctx, parent: &parent}
			}
			r := qualifyingCaptureReceipt(m, hash)
			r.Complete, r.MaterialHash = false, ""
			e = finishCapture(actionContext, &r, m.CapturePath, &parent, raw, func(p *captureParentIdentity, path string, data []byte) error {
				if e := writeCaptureExclusive(p, path, data); e != nil {
					return e
				}
				switch failure {
				case "after_write":
					cancel()
				case "closed_parent":
					return p.directory.Close()
				}
				return nil
			})
			if e == nil || r.MaterialHash != "" {
				t.Fatal("failed capture was certified")
			}
			r.Error = e.Error()
			remaining, e := os.ReadFile(m.CapturePath)
			if e != nil || !bytes.Equal(remaining, raw) || fingerprint(remaining) != hash {
				t.Fatal("failure diagnostics must retain exact original material")
			}
			var stdout bytes.Buffer
			exit := emitReceipt(r, &stdout)
			if exit == 0 {
				t.Fatal("failed capture emitted success exit")
			}
			path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), "failed", stdout.Bytes(), exit)
			for _, action := range []string{"verify-target", "restore"} {
				args := actorConsumerArgs(m, hash, action, approvalPath, approvalHash)
				for _, proof := range [][]string{nil, {"--capture-evidence-file=" + path, "--capture-evidence-sha256=" + evidenceHash}} {
					result := run(t.Context(), append(args, proof...), func(string) string { t.Fatal("failed capture accessed database configuration"); return "" }, fixtureSource)
					if result.Complete || result.ApprovalConsumed || result.SQLPoolClosed || result.MongoClosed {
						t.Fatal("failed capture remainder crossed the consumer boundary", result)
					}
				}
			}
		})
	}
}

func TestCaptureEvidenceRequiresOriginalSuccessAndExactScope(t *testing.T) {
	m, _, hash, ap, ah := recoveryInputs(t)
	good := qualifyingCaptureReceipt(m, hash)
	for name, change := range map[string]func(*receipt){
		"incomplete":          func(r *receipt) { r.Complete = false },
		"wrong_action":        func(r *receipt) { r.Action = "verify-target" },
		"wrong_material":      func(r *receipt) { r.MaterialHash = fingerprint([]byte("different original")) },
		"wrong_source":        func(r *receipt) { r.ToolSource = "0000000000000000000000000000000000000000" },
		"unreleased":          func(r *receipt) { r.MongoClosed = false },
		"unknown_transaction": func(r *receipt) { r.TransactionReleaseUnproven = true },
		"wrong_scope":         func(r *receipt) { r.TableCounts = map[string]int{} },
		"budget_exceeded":     func(r *receipt) { r.BudgetExceeded = true },
	} {
		t.Run(name, func(t *testing.T) {
			r := good
			change(&r)
			raw, _ := json.Marshal(r)
			path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), name, raw, 0)
			if readCaptureEvidence(m, hash, fixtureSource, path, evidenceHash) == nil {
				t.Fatal("unsuccessful or unrelated capture proof accepted")
			}
		})
	}
	args := actorConsumerArgs(m, hash, "restore", ap, ah)
	args = append(args, unitCaptureEvidence(t, m, hash, "valid")...)
	calls := 0
	r := run(t.Context(), args, func(string) string { calls++; return "" }, fixtureSource)
	if calls != 2 || r.Error != "selected_connection_input_rejected" {
		t.Fatal("qualified capture did not reach the original connection-input boundary", r)
	}
}

type fullReceiptErrorWriter struct{ bytes.Buffer }

func (w *fullReceiptErrorWriter) Write(p []byte) (int, error) {
	n, _ := w.Buffer.Write(p)
	return n, errors.New("output acknowledgement failed")
}

func TestSuccessfulCaptureJSONWithFailedOutputExitIsRejected(t *testing.T) {
	m, _, hash, _, _ := recoveryInputs(t)
	w := &fullReceiptErrorWriter{}
	exit := emitReceipt(qualifyingCaptureReceipt(m, hash), w)
	if exit == 0 {
		t.Fatal("output failure returned success")
	}
	path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), "output-failed", w.Bytes(), exit)
	if readCaptureEvidence(m, hash, fixtureSource, path, evidenceHash) == nil {
		t.Fatal("valid success JSON hid the actual failed process exit")
	}
}

func TestCaptureDocumentsRejectAliasesMissingMembersAndNullCounts(t *testing.T) {
	m, _, _, _, _ := recoveryInputs(t)
	m.Tables[0].Rows = nil
	m.Tables[0].Pages = 1
	if validateMaterial(m, fixtureSource, m.Binding) != nil {
		t.Fatal("valid zero-row material fixture rejected")
	}
	rawMaterial, _ := json.Marshal(m)
	hash := fingerprint(rawMaterial)
	rawReceipt, _ := json.Marshal(qualifyingCaptureReceipt(m, hash))
	for _, kind := range []string{"case_alias", "duplicate_alias", "missing_false", "null_false", "null_zero_count"} {
		t.Run(kind, func(t *testing.T) {
			var fields map[string]any
			if json.Unmarshal(rawReceipt, &fields) != nil {
				t.Fatal("decode unit receipt")
			}
			switch kind {
			case "case_alias":
				fields["Complete"] = fields["complete"]
				delete(fields, "complete")
			case "duplicate_alias":
				fields["Complete"] = true
			case "missing_false":
				delete(fields, "total_budget_exceeded")
			case "null_false":
				fields["total_budget_exceeded"] = nil
			case "null_zero_count":
				fields["table_rows"].(map[string]any)[m.Tables[0].Table] = nil
			}
			changed, _ := json.Marshal(fields)
			path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), kind, changed, 0)
			if readCaptureEvidence(m, hash, fixtureSource, path, evidenceHash) == nil {
				t.Fatal("non-original stdout document accepted", kind)
			}
		})
	}
	var stdout bytes.Buffer
	if emitReceipt(qualifyingCaptureReceipt(m, hash), &stdout) != 0 {
		t.Fatal("valid zero-row original receipt encoding failed")
	}
	path, evidenceHash := freezeCaptureExecution(t, filepath.Dir(m.CapturePath), "zero-rows", stdout.Bytes(), 0)
	if readCaptureEvidence(m, hash, fixtureSource, path, evidenceHash) != nil {
		t.Fatal("actual numeric zero-row success receipt rejected")
	}
	raw, e := readPrivateLimit(path, evidenceHash, 32768)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"case_exit", "missing_exit", "null_exit", "unknown_termination"} {
		t.Run(kind, func(t *testing.T) {
			var fields map[string]any
			if json.Unmarshal(raw, &fields) != nil {
				t.Fatal("decode unit execution evidence")
			}
			switch kind {
			case "case_exit":
				fields["Exit_Code"] = fields["exit_code"]
				delete(fields, "exit_code")
			case "missing_exit":
				delete(fields, "exit_code")
			case "null_exit":
				fields["exit_code"] = nil
			case "unknown_termination":
				fields["termination"] = "unknown"
			}
			changed, _ := json.Marshal(fields)
			proof := filepath.Join(filepath.Dir(path), kind+".json")
			if writeExclusive(proof, changed) != nil || readCaptureEvidence(m, hash, fixtureSource, proof, fingerprint(changed)) == nil {
				t.Fatal("execution was not a proven original exit0", kind)
			}
		})
	}
}
