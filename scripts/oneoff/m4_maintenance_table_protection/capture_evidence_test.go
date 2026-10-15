package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit-only successful receipt value. Native tests instead preserve the actual
// capture serve output and return status from the owned database invocation.
func unitCaptureReceipt(m material, hash string) receipt {
	r := receipt{Action: "capture", ToolSource: m.ToolSource, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, Complete: true, ReadOnly: true, MaterialHash: hash, Error: "none", SQLSnapshot: true, MandatoryEOF: true, RetainedEOF: true, SQLConnectionClosed: true, SQLPoolClosed: true, MongoClosed: true, TableCounts: map[string]int{}, RetainedCounts: map[string]int{}}
	r.DatabaseBoundary = &m.RuntimeBoundary
	for _, table := range m.Tables {
		r.TableCounts[table.Table] = len(table.Rows)
	}
	for _, table := range m.Retained {
		r.RetainedCounts[table.Table] = len(table.Rows)
	}
	return r
}

func unitCaptureEvidence(t *testing.T, r receipt, exit int) (string, string, string) {
	t.Helper()
	dir := privateFixture(t)
	receiptPath := filepath.Join(dir, "original-receipt.json")
	var stdout bytes.Buffer
	if json.NewEncoder(&stdout).Encode(r) != nil || writeExclusive(receiptPath, stdout.Bytes()) != nil {
		t.Fatal("unit original receipt unavailable")
	}
	proof := captureExecutionEvidence{Format: 1, Kind: "m4_d_capture_execution", Termination: "exited", ExitCode: &exit, ReceiptFile: receiptPath, ReceiptHash: fingerprint(stdout.Bytes())}
	raw, e := json.Marshal(proof)
	path := filepath.Join(dir, "execution.json")
	if e != nil || writeExclusive(path, raw) != nil {
		t.Fatal("unit execution evidence unavailable")
	}
	return path, fingerprint(raw), receiptPath
}

func unitCaptureEvidenceArgs(t *testing.T, m material, hash string) []string {
	t.Helper()
	path, evidenceHash, _ := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
	if readCaptureEvidence(m, hash, testTool, path, evidenceHash) != nil {
		t.Fatal("unit qualifying capture evidence rejected before intended negative test")
	}
	return []string{"--capture-evidence-file=" + path, "--capture-evidence-sha256=" + evidenceHash}
}

func materialConsumerArgs(t *testing.T, action string, m material, a approval, hash, approvalPath, approvalHash string) []string {
	t.Helper()
	args := append(cliArgs(action), "--material-file="+m.CapturePath, "--material-sha256="+hash)
	if action == "restore" || action == "verify-restored" {
		args = append(args, "--approval-file="+approvalPath, "--approval-sha256="+approvalHash)
	}
	if action == "mark-first-runtime" {
		v := fixtureRuntimeIntent(m, hash, a)
		raw, e := json.Marshal(v)
		path := filepath.Join(privateFixture(t), "intent.json")
		if e != nil || writeExclusive(path, raw) != nil {
			t.Fatal("unit independent intent unavailable")
		}
		args = append(args, "--runtime-intent-file="+path, "--runtime-intent-sha256="+fingerprint(raw))
	}
	return args
}

func TestFailedCaptureLeftoverCannotBecomeRecoveryMaterial(t *testing.T) {
	for _, failure := range []string{"cancel_after_write", "close_after_write", "budget_after_close"} {
		t.Run(failure, func(t *testing.T) {
			m, a, hash, ap, ah := recoveryInputs(t)
			raw, e := os.ReadFile(m.CapturePath)
			if e != nil || os.Remove(m.CapturePath) != nil {
				t.Fatal("unit original material unavailable")
			}
			parent, e := captureParent(m.CapturePath)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = parent.close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			captureContext := ctx
			if failure == "budget_after_close" {
				captureContext = captureReleasedContext{Context: ctx, parent: &parent}
			}
			r := unitCaptureReceipt(m, hash)
			r.Complete, r.MaterialHash = false, ""
			e = finishCapture(captureContext, &r, m.CapturePath, &parent, raw, func(p *captureParentIdentity, path string, value []byte) error {
				if e := writeCaptureExclusive(p, path, value); e != nil {
					return e
				}
				switch failure {
				case "cancel_after_write":
					cancel()
				case "close_after_write":
					return p.directory.Close()
				}
				return nil
			})
			if e == nil {
				t.Fatal("actual late capture failure not reached")
			}
			r.Error = e.Error()
			leftover, e := readPrivate(m.CapturePath, hash)
			if e != nil || !bytes.Equal(leftover, raw) || fingerprint(leftover) != hash || validateApproval(a, m, hash, testTool, mustTime(a.RecoveryDecision)) != nil {
				t.Fatal("complete leftover and otherwise valid independent approval not reproduced")
			}
			for _, claimedExit := range []int{1, 0} {
				// A false externally supplied exit0 still cannot turn the actual
				// unsuccessful capture receipt into a successful one.
				proof, proofHash, _ := unitCaptureEvidence(t, r, claimedExit)
				category := "original_capture_execution_evidence_rejected"
				if claimedExit == 0 {
					category = "original_capture_success_receipt_rejected"
				}
				for _, action := range []string{"verify-target", "restore", "verify-restored", "mark-first-runtime"} {
					args := materialConsumerArgs(t, action, m, a, hash, ap, ah)
					args = append(args, "--capture-evidence-file="+proof, "--capture-evidence-sha256="+proofHash)
					got := run(context.Background(), args, func(string) string { t.Fatal("failed capture reached ENV", action); return "" }, testTool)
					if got.Complete || got.Error != category || got.ApprovalConsumed || got.RuntimeGateAcquired || got.MarkerWritten {
						t.Fatal("failed capture became eligible", action, claimedExit, got)
					}
				}
			}
			after, e := os.ReadFile(m.CapturePath)
			if e != nil || !bytes.Equal(after, raw) {
				t.Fatal("failed material was deleted or changed")
			}
		})
	}
}

func TestCaptureReceiptSuccessMustMatchCompleteOriginalScope(t *testing.T) {
	m := originalMaterial()
	hash := fingerprint([]byte("unit original material bytes"))
	if validateCaptureReceipt(unitCaptureReceipt(m, hash), m, hash, testTool) != nil {
		t.Fatal("complete matching original capture receipt rejected")
	}
	changes := map[string]func(*receipt){
		"action":           func(r *receipt) { r.Action = "verify-target" },
		"complete":         func(r *receipt) { r.Complete = false },
		"error":            func(r *receipt) { r.Error = "total_budget_exceeded" },
		"hash":             func(r *receipt) { r.MaterialHash = fingerprint([]byte("other")) },
		"tool":             func(r *receipt) { r.ToolSource = strings.Repeat("2", 40) },
		"source":           func(r *receipt) { r.DSource = testTool },
		"resources":        func(r *receipt) { r.DResources = hash },
		"resource_count":   func(r *receipt) { r.DResourceFiles-- },
		"readonly":         func(r *receipt) { r.ReadOnly = false },
		"snapshot":         func(r *receipt) { r.SQLSnapshot = false },
		"eof":              func(r *receipt) { r.MandatoryEOF = false },
		"retained_eof":     func(r *receipt) { r.RetainedEOF = false },
		"connection":       func(r *receipt) { r.SQLConnectionClosed = false },
		"pool":             func(r *receipt) { r.SQLPoolClosed = false },
		"mongo":            func(r *receipt) { r.MongoClosed = false },
		"release_error":    func(r *receipt) { r.ReleaseError = "release_unproven" },
		"transaction":      func(r *receipt) { r.TransactionReleaseUnproven = true },
		"budget":           func(r *receipt) { r.BudgetExceeded = true },
		"extra_table":      func(r *receipt) { r.TableCounts["other"] = 0 },
		"missing_table":    func(r *receipt) { delete(r.TableCounts, scope[0].name) },
		"table_count":      func(r *receipt) { r.TableCounts[scope[0].name]++ },
		"retained_count":   func(r *receipt) { r.RetainedCounts[retainedScope[0].name]-- },
		"missing_retained": func(r *receipt) { r.RetainedCounts = nil },
		"freeze_claim":     func(r *receipt) { r.ProductionFreezeCertified = true },
		"atomic_claim":     func(r *receipt) { r.CrossDatabaseAtomicRR = true },
		"approval":         func(r *receipt) { r.ApprovalConsumed = true },
		"gate":             func(r *receipt) { r.RuntimeGateAcquired = true },
		"gate_release":     func(r *receipt) { r.RuntimeGateReleased = true },
		"gate_unknown":     func(r *receipt) { r.RuntimeGateReleaseUnproven = true },
		"marker":           func(r *receipt) { r.MarkerWritten = true },
		"marker_unknown":   func(r *receipt) { r.MarkerWriteUnknown = true },
		"marker_hash":      func(r *receipt) { r.MarkerHash = hash },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			r := unitCaptureReceipt(m, hash)
			change(&r)
			if validateCaptureReceipt(r, m, hash, testTool) == nil {
				t.Fatal("unsuccessful or rebound capture receipt admitted")
			}
		})
	}
}

type retainedOutputFailure struct{ bytes.Buffer }

func (out *retainedOutputFailure) Write(raw []byte) (int, error) {
	n, _ := out.Buffer.Write(raw)
	return n, errors.New("original stdout delivery failed after write")
}

func TestSuccessJSONDoesNotReplaceOriginalZeroExit(t *testing.T) {
	m, _, hash, _, _ := recoveryInputs(t)
	var out retainedOutputFailure
	exit := emitReceipt(unitCaptureReceipt(m, hash), &out)
	var r receipt
	if exit != 1 || strictDecode(out.Bytes(), &r) != nil || !r.Complete {
		t.Fatal("actual CLI delivery boundary did not retain success JSON with nonzero exit")
	}
	path, evidenceHash, receiptPath := unitCaptureEvidence(t, r, exit)
	// Preserve exactly the bytes delivered by the production receipt emitter.
	if os.WriteFile(receiptPath, out.Bytes(), 0600) != nil {
		t.Fatal("unit retained stdout unavailable")
	}
	if readCaptureEvidence(m, hash, testTool, path, evidenceHash) == nil {
		t.Fatal("success JSON overrode the original nonzero process result")
	}
}

func TestEveryMaterialConsumerRequiresOriginalCaptureEvidenceBeforeEnvironment(t *testing.T) {
	m, a, hash, ap, ah := recoveryInputs(t)
	for _, action := range []string{"verify-target", "restore", "verify-restored", "mark-first-runtime"} {
		args := materialConsumerArgs(t, action, m, a, hash, ap, ah)
		r := run(context.Background(), args, func(string) string { t.Fatal("missing capture evidence reached ENV"); return "" }, testTool)
		if r.Complete || r.Error != "original_capture_evidence_required" || r.ApprovalConsumed || r.RuntimeGateAcquired {
			t.Fatal("missing evidence admitted", action, r)
		}
	}
	for _, action := range []string{"inspect", "capture"} {
		base := cliArgs(action)
		if action == "capture" {
			base = append(base, "--capture-file="+filepath.Join(privateFixture(t), "new-material.json"))
		}
		for _, proof := range [][]string{unitCaptureEvidenceArgs(t, m, hash), {"--capture-evidence-file="}, {"--capture-evidence-sha256="}} {
			args := append(append([]string(nil), base...), proof...)
			if _, e := parseArgs(args); e == nil || e.Error() != "capture_evidence_action_mismatch" {
				t.Fatal("capture evidence leaked into unrelated action", action, e)
			}
		}
	}
}

func TestCaptureExecutionAndOriginalReceiptAreStrictPrivateFrozenInputs(t *testing.T) {
	for _, name := range []string{"missing_exit", "null_exit", "nonzero_exit", "signal", "unknown_termination", "wrong_kind", "unknown_member", "duplicate_member", "trailing_document", "execution_limit", "receipt_limit", "execution_sha", "receipt_sha", "receipt_unknown_member", "receipt_duplicate_member", "receipt_trailing_document", "receipt_incomplete", "receipt_error", "receipt_hash", "receipt_tool", "receipt_scope", "receipt_release", "execution_mode", "receipt_mode", "execution_symlink", "receipt_symlink", "same_receipt_and_execution", "receipt_is_material"} {
		t.Run(name, func(t *testing.T) {
			m, a, hash, ap, ah := recoveryInputs(t)
			path, evidenceHash, receiptPath := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
			if readCaptureEvidence(m, hash, testTool, path, evidenceHash) != nil {
				t.Fatal("initial complete private capture evidence rejected")
			}
			raw, _ := os.ReadFile(path)
			var proof captureExecutionEvidence
			if strictDecode(raw, &proof) != nil {
				t.Fatal("unit original execution unavailable")
			}
			r := unitCaptureReceipt(m, hash)
			switch name {
			case "missing_exit":
				raw = bytes.Replace(raw, []byte(`"exit_code":0,`), nil, 1)
			case "null_exit":
				raw = bytes.Replace(raw, []byte(`"exit_code":0`), []byte(`"exit_code":null`), 1)
			case "nonzero_exit":
				one := 1
				proof.ExitCode = &one
			case "signal", "unknown_termination":
				proof.Termination = name
			case "wrong_kind":
				proof.Kind = "m4_other_capture_execution"
			case "unknown_member":
				raw = append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...)
			case "duplicate_member":
				raw = append(raw[:len(raw)-1], []byte(`,"exit_code":0}`)...)
			case "trailing_document":
				raw = append(raw, []byte(` {}`)...)
			case "execution_limit":
				raw = bytes.Repeat([]byte{' '}, 32769)
			case "receipt_limit":
				if os.WriteFile(receiptPath, bytes.Repeat([]byte{' '}, 65537), 0600) != nil {
					t.Fatal("oversized original stdout unavailable")
				}
			case "execution_sha":
				evidenceHash = fingerprint([]byte("wrong envelope bytes"))
			case "receipt_sha":
				proof.ReceiptHash = fingerprint([]byte("wrong stdout bytes"))
			case "receipt_unknown_member", "receipt_duplicate_member", "receipt_trailing_document":
				stdout, _ := os.ReadFile(receiptPath)
				stdout = bytes.TrimSpace(stdout)
				switch name {
				case "receipt_unknown_member":
					stdout = append(stdout[:len(stdout)-1], []byte(`,"unknown":true}`)...)
				case "receipt_duplicate_member":
					stdout = append(stdout[:len(stdout)-1], []byte(`,"complete":true}`)...)
				case "receipt_trailing_document":
					stdout = append(stdout, []byte(` {}`)...)
				}
				if os.WriteFile(receiptPath, stdout, 0600) != nil {
					t.Fatal("invalid original stdout unavailable")
				}
				proof.ReceiptHash = fingerprint(stdout)
			case "receipt_incomplete":
				r.Complete = false
			case "receipt_error":
				r.Error = "total_budget_exceeded"
			case "receipt_hash":
				r.MaterialHash = fingerprint([]byte("different material"))
			case "receipt_tool":
				r.ToolSource = strings.Repeat("3", 40)
			case "receipt_scope":
				r.TableCounts[scope[0].name]++
			case "receipt_release":
				r.SQLConnectionClosed = false
			case "execution_mode":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("unit mode change unavailable")
				}
			case "receipt_mode":
				if os.Chmod(receiptPath, 0644) != nil {
					t.Fatal("unit mode change unavailable")
				}
			case "execution_symlink", "receipt_symlink":
				from := path
				if name == "receipt_symlink" {
					from = receiptPath
				}
				if os.Rename(from, from+".original") != nil || os.Symlink(from+".original", from) != nil {
					t.Fatal("unit symlink unavailable")
				}
			case "same_receipt_and_execution":
				proof.ReceiptFile = path
			case "receipt_is_material":
				proof.ReceiptFile, proof.ReceiptHash = m.CapturePath, hash
			}
			if strings.HasPrefix(name, "receipt_") && (name == "receipt_incomplete" || name == "receipt_error" || name == "receipt_hash" || name == "receipt_tool" || name == "receipt_scope" || name == "receipt_release") {
				stdout, _ := json.Marshal(r)
				if os.WriteFile(receiptPath, stdout, 0600) != nil {
					t.Fatal("unit unsuccessful stdout unavailable")
				}
				proof.ReceiptHash = fingerprint(stdout)
			}
			switch name {
			case "nonzero_exit", "signal", "unknown_termination", "wrong_kind", "receipt_sha", "receipt_unknown_member", "receipt_duplicate_member", "receipt_trailing_document", "receipt_incomplete", "receipt_error", "receipt_hash", "receipt_tool", "receipt_scope", "receipt_release", "same_receipt_and_execution", "receipt_is_material":
				raw, _ = json.Marshal(proof)
			}
			if name != "execution_symlink" {
				if os.WriteFile(path, raw, 0600) != nil {
					t.Fatal("unit execution mutation unavailable")
				}
			}
			if name != "execution_sha" {
				evidenceHash = fingerprint(raw)
			}
			for _, action := range []string{"verify-target", "restore", "verify-restored", "mark-first-runtime"} {
				args := materialConsumerArgs(t, action, m, a, hash, ap, ah)
				args = append(args, "--capture-evidence-file="+path, "--capture-evidence-sha256="+evidenceHash)
				got := run(context.Background(), args, func(string) string { t.Fatal("invalid capture evidence reached ENV", action); return "" }, testTool)
				if got.Complete || !strings.HasPrefix(got.Error, "original_capture_") || got.ApprovalConsumed || got.RuntimeGateAcquired || got.MarkerWritten {
					t.Fatal("invalid original execution admitted", action, got)
				}
			}
		})
	}
}

func TestRuntimeLockedInputRereadRejectsChangedCaptureEvidence(t *testing.T) {
	for _, changed := range []string{"execution", "original_stdout"} {
		t.Run(changed, func(t *testing.T) {
			m, ap, hash, _, _ := recoveryInputs(t)
			v := fixtureRuntimeIntent(m, hash, ap)
			raw, _ := json.Marshal(v)
			intentPath := filepath.Join(privateFixture(t), "intent.json")
			if writeExclusive(intentPath, raw) != nil {
				t.Fatal("unit runtime intent unavailable")
			}
			path, evidenceHash, receiptPath := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
			a := arguments{material: m.CapturePath, materialHash: hash, captureEvidence: path, captureEvidenceHash: evidenceHash, runtimeIntent: intentPath, runtimeIntentHash: fingerprint(raw)}
			if rereadRuntimeInputs(v, m, a, testTool, nil) != nil {
				t.Fatal("initial complete locked input rejected")
			}
			changedPath := path
			if changed == "original_stdout" {
				changedPath = receiptPath
			}
			before, _ := os.ReadFile(changedPath)
			if os.WriteFile(changedPath, append(before, '\n'), 0600) != nil {
				t.Fatal("unit raw evidence mutation unavailable")
			}
			if rereadRuntimeInputs(v, m, a, testTool, nil) == nil {
				t.Fatal("locked author ignored original frozen evidence mutation")
			}
		})
	}
}

func TestCaptureDocumentsRequireOriginalLiteralTypedMembers(t *testing.T) {
	type mutation struct {
		receipt bool
		change  func(map[string]any)
	}
	changes := map[string]mutation{
		"complete_alias":       {true, func(v map[string]any) { v["Complete"] = v["complete"]; delete(v, "complete") }},
		"complete_alias_pair":  {true, func(v map[string]any) { v["Complete"] = v["complete"] }},
		"missing_false_risk":   {true, func(v map[string]any) { delete(v, "runtime_marker_write_unknown") }},
		"null_false_risk":      {true, func(v map[string]any) { v["runtime_marker_write_unknown"] = nil }},
		"missing_false_freeze": {true, func(v map[string]any) { delete(v, "production_freeze_certified") }},
		"null_false_freeze":    {true, func(v map[string]any) { v["production_freeze_certified"] = nil }},
		"null_zero_row_count":  {true, func(v map[string]any) { v["table_rows"].(map[string]any)[scope[0].name] = nil }},
		"exit_alias":           {false, func(v map[string]any) { v["Exit_Code"] = v["exit_code"]; delete(v, "exit_code") }},
		"exit_alias_pair":      {false, func(v map[string]any) { v["Exit_Code"] = v["exit_code"] }},
		"missing_exit":         {false, func(v map[string]any) { delete(v, "exit_code") }},
		"null_exit":            {false, func(v map[string]any) { v["exit_code"] = nil }},
	}
	for name, mutate := range changes {
		t.Run(name, func(t *testing.T) {
			m, _, _, _, _ := recoveryInputs(t)
			// A real empty table makes null->int(0) otherwise indistinguishable
			// to the typed count validator. Keep the rest of the material valid.
			m.Tables[0].Rows, m.Tables[0].Pages, m.Tables[0].EncodedRowsBytes = nil, 1, 0
			materialRaw, _ := json.Marshal(m)
			hash := fingerprint(materialRaw)
			if validateMaterial(m, testTool, m.Binding) != nil || os.WriteFile(m.CapturePath, materialRaw, 0600) != nil {
				t.Fatal("valid empty-table material unavailable")
			}
			path, evidenceHash, receiptPath := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
			if readCaptureEvidence(m, hash, testTool, path, evidenceHash) != nil {
				t.Fatal("literal original documents not admitted")
			}
			changedPath := path
			if mutate.receipt {
				changedPath = receiptPath
			}
			raw, _ := os.ReadFile(changedPath)
			var tree map[string]any
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if decoder.Decode(&tree) != nil {
				t.Fatal("unit original tree unavailable")
			}
			mutate.change(tree)
			raw, _ = json.MarshalIndent(tree, "", "  ")
			if os.WriteFile(changedPath, raw, 0600) != nil {
				t.Fatal("unit literal mutation unavailable")
			}
			if mutate.receipt {
				envelopeRaw, _ := os.ReadFile(path)
				var proof captureExecutionEvidence
				if strictDecode(envelopeRaw, &proof) != nil {
					t.Fatal("unit original execution unavailable")
				}
				proof.ReceiptHash = fingerprint(raw)
				envelopeRaw, _ = json.Marshal(proof)
				if os.WriteFile(path, envelopeRaw, 0600) != nil {
					t.Fatal("unit mutated receipt binding unavailable")
				}
				evidenceHash = fingerprint(envelopeRaw)
			} else {
				evidenceHash = fingerprint(raw)
			}
			if readCaptureEvidence(m, hash, testTool, path, evidenceHash) == nil {
				t.Fatal("non-original members or defaulted null/omission admitted")
			}
		})
	}
}
