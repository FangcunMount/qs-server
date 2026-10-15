package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureEvidenceArgs(input captureEvidenceInput) []string {
	return []string{"--capture-evidence-file=" + input.path, "--capture-evidence-sha256=" + input.hash}
}

func fixtureCaptureReceipt(m material, materialHash string) receipt {
	return receipt{
		Action: "capture", ToolSource: m.ToolSource, BSource: m.BSource, BResources: m.BResources, BResourceFiles: m.BResourceFiles,
		Complete: true, ReadOnly: true, MaterialHash: materialHash, SQLIdentity: m.SQLIdentity, MongoIdentity: m.MongoIdentity, SQLVersion: m.SQLVersion, MongoVersion: 39,
		Counts: [2]*int64{m.Collections[0].Count, m.Collections[1].Count}, SQLConnectionClosed: boolean(true), SQLPoolClosed: boolean(true), MongoClosed: boolean(true), MongoCursorsClosed: boolean(true), SessionEndCalled: true, Error: "none",
	}
}

// Unit-only synthetic receipt encoder. Native capture instead freezes the
// original stdout and exit returned by actual serve via freezeCaptureProcess.
func freezeCaptureReceipt(t *testing.T, dir string, r receipt) captureEvidenceInput {
	t.Helper()
	var stdout bytes.Buffer
	exit := deliverReceipt(r, &stdout)
	if exit != 0 {
		t.Fatal("fixture capture did not complete through the CLI receipt encoder")
	}
	return freezeCaptureProcess(t, dir, stdout.Bytes(), "exited", &exit)
}

func freezeCaptureProcess(t *testing.T, dir string, stdout []byte, termination string, exit *int) captureEvidenceInput {
	t.Helper()
	receiptPath := filepath.Join(dir, "capture-stdout.json")
	if e := writeExclusive(receiptPath, stdout); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(captureExecution{1, "m4_b_capture_execution", receiptPath, fingerprint(stdout), termination, exit})
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "capture-execution.json")
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	return captureEvidenceInput{path, fingerprint(raw)}
}

func fixtureCaptureEvidence(t *testing.T, m material, materialHash string) captureEvidenceInput {
	t.Helper()
	return freezeCaptureReceipt(t, privateDir(t), fixtureCaptureReceipt(m, materialHash))
}

func TestFailedPersistedCaptureCannotBecomeApprovedRecoveryMaterial(t *testing.T) {
	m := fixtureMaterial(t)
	m.CapturedAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	dir := privateDir(t)
	m.CapturePath = filepath.Join(dir, "failed-material.json")
	raw, _ := json.Marshal(m)
	parent, e := captureParent(m.CapturePath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = parent.close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := fixtureCaptureReceipt(m, "")
	r.Complete, r.Error = false, "capture_in_progress"
	e = finishCapture(ctx, &r, m.CapturePath, &parent, raw, func(parent *captureParentIdentity, path string, raw []byte) error {
		if e := writeCaptureExclusive(parent, path, raw); e != nil {
			return e
		}
		cancel()
		return nil
	})
	if e == nil || e.Error() != "total_budget_exceeded" || r.MaterialHash != "" || r.Complete {
		t.Fatal("real after-persistence failure was not retained", e, r)
	}
	r.Error, r.BudgetExceeded = e.Error(), true
	manualHash := fingerprint(raw)
	if got, e := readPrivate(m.CapturePath, manualHash, maxMaterialBytes); e != nil || !bytes.Equal(got, raw) || validateMaterial(m, testSource, m.SQLName, m.MongoName) != nil {
		t.Fatal("failure did not leave the complete original material")
	}
	a, approvalPath, approvalHash := freezeRestoreApproval(t, m, manualHash, dir)
	if e := validateApproval(a, m, manualHash, testSource, time.Now()); e != nil {
		t.Fatal("independent restore approval must otherwise be valid", e)
	}
	var stdout bytes.Buffer
	exit := deliverReceipt(r, &stdout)
	if exit != 1 {
		t.Fatal("failed capture was reported as a successful process")
	}
	proof := freezeCaptureProcess(t, privateDir(t), stdout.Bytes(), "exited", &exit)
	for _, action := range []string{"verify-target", "restore", "verify-restored"} {
		t.Run(action, func(t *testing.T) {
			args := append([]string{"--action=" + action, "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + m.CapturePath, "--material-sha256=" + manualHash}, captureEvidenceArgs(proof)...)
			if action != "verify-target" {
				args = append(args, "--approval-file="+approvalPath, "--approval-sha256="+approvalHash)
			}
			got := run(context.Background(), args, func(string) string {
				t.Fatal("failed original capture reached connection inputs")
				return ""
			}, testSource, nil)
			if got.Complete || got.Error != "capture_execution_evidence_rejected" || got.SQLConnectionClosed != nil || got.SQLPoolClosed != nil || got.MongoClosed != nil {
				t.Fatal("manual material hash and valid approval revived failed capture", got)
			}
			if after, _ := os.ReadFile(m.CapturePath); !bytes.Equal(after, raw) {
				t.Fatal("failed material was deleted or rewritten")
			}
		})
	}
	// Even an incorrect executor exit0 cannot turn the failed stdout into a
	// successful capture. The tool requires both independent exit and receipt.
	zero := 0
	proof = freezeCaptureProcess(t, privateDir(t), stdout.Bytes(), "exited", &zero)
	if e := proof.check(m, manualHash, testSource); e == nil || e.Error() != "capture_success_receipt_rejected" {
		t.Fatal("exit0 replaced the original failed capture result", e)
	}
}

func TestCaptureExecutionRequiresCompleteOriginalExitAndStrictEnvelope(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	for _, fault := range []string{"valid", "nonzero_exit", "null_exit", "missing_exit", "signaled", "wrong_kind", "wrong_format", "receipt_hash", "relative_receipt", "unknown_member", "duplicate_member", "case_alias", "null_format", "oversize"} {
		t.Run(fault, func(t *testing.T) {
			proof := fixtureCaptureEvidence(t, m, hash)
			original, _ := os.ReadFile(proof.path)
			var fields map[string]any
			if json.Unmarshal(original, &fields) != nil {
				t.Fatal("fixture execution document")
			}
			switch fault {
			case "nonzero_exit":
				fields["exit_code"] = 1
			case "null_exit":
				fields["exit_code"] = nil
			case "missing_exit":
				delete(fields, "exit_code")
			case "signaled":
				fields["termination"] = "signaled"
			case "wrong_kind":
				fields["kind"] = "m4_a_capture_execution"
			case "wrong_format":
				fields["format_version"] = 2
			case "receipt_hash":
				fields["receipt_sha256"] = strings.Repeat("e", 64)
			case "relative_receipt":
				fields["receipt_file"] = "capture-stdout.json"
			case "unknown_member":
				fields["complete"] = true
			case "null_format":
				fields["format_version"] = nil
			case "case_alias":
				fields["exit_code"] = 1
			}
			raw, _ := json.Marshal(fields)
			if fault == "duplicate_member" {
				raw = append(raw[:len(raw)-1], []byte(`,"exit_code":0}`)...)
			}
			if fault == "case_alias" {
				raw = append(raw[:len(raw)-1], []byte(`,"Exit_Code":0}`)...)
			}
			if fault == "oversize" {
				raw = append(raw, bytes.Repeat([]byte(" "), maxCaptureExecutionBytes)...)
			}
			if os.WriteFile(proof.path, raw, 0600) != nil {
				t.Fatal("could not mutate owned evidence")
			}
			proof.hash = fingerprint(raw)
			e := proof.check(m, hash, testSource)
			if fault == "valid" && e != nil || fault != "valid" && e == nil {
				t.Fatal("executor status or original stdout binding not enforced", fault, e)
			}
		})
	}
}

func TestCaptureReceiptBindsObservedBCountsScopeAndAllReleaseResponsibilities(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	mutations := map[string]func(*receipt){
		"wrong_action":      func(r *receipt) { r.Action = "verify-target" },
		"incomplete":        func(r *receipt) { r.Complete = false },
		"failed":            func(r *receipt) { r.Error = "total_budget_exceeded" },
		"not_readonly":      func(r *receipt) { r.ReadOnly = false },
		"production_claim":  func(r *receipt) { r.FreezeCertified = true },
		"material_hash":     func(r *receipt) { r.MaterialHash = strings.Repeat("d", 64) },
		"tool":              func(r *receipt) { r.ToolSource = bSource },
		"source":            func(r *receipt) { r.BSource = testSource },
		"resources":         func(r *receipt) { r.BResources = strings.Repeat("d", 64) },
		"resource_count":    func(r *receipt) { r.BResourceFiles++ },
		"sql_identity":      func(r *receipt) { r.SQLIdentity = strings.Repeat("d", 64) },
		"mongo_identity":    func(r *receipt) { r.MongoIdentity = strings.Repeat("d", 64) },
		"sql_version":       func(r *receipt) { r.SQLVersion = 100 },
		"mongo_version":     func(r *receipt) { r.MongoVersion = 40 },
		"missing_count":     func(r *receipt) { r.Counts[0] = nil },
		"count":             func(r *receipt) { n := int64(18); r.Counts[1] = &n },
		"sql_connection":    func(r *receipt) { r.SQLConnectionClosed = boolean(false) },
		"sql_pool":          func(r *receipt) { r.SQLPoolClosed = nil },
		"mongo_connection":  func(r *receipt) { r.MongoClosed = boolean(false) },
		"cursor_eof":        func(r *receipt) { r.MongoCursorsClosed = boolean(false) },
		"snapshot_session":  func(r *receipt) { r.SessionEndCalled = false },
		"sql_transaction":   func(r *receipt) { r.TransactionReleaseUnproven = true },
		"mongo_transaction": func(r *receipt) { r.MongoTransactionReleaseUnproven = true },
		"commit_issued":     func(r *receipt) { r.LedgerCommitIssued = true },
		"commit_ack":        func(r *receipt) { r.LedgerCommitAcknowledged = true },
		"commit_unknown":    func(r *receipt) { r.LedgerCommitOutcomeUnknown = true },
		"abort":             func(r *receipt) { r.LedgerAbortCalled = true },
		"release_error":     func(r *receipt) { r.ReleaseError = "owned_resource_release_unproven" },
		"budget":            func(r *receipt) { r.BudgetExceeded = true },
		"restored_proof":    func(r *receipt) { r.RestoredVerification = &restoredVerification{} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := fixtureCaptureReceipt(m, hash)
			mutate(&r)
			var stdout bytes.Buffer
			_ = deliverReceipt(r, &stdout)
			zero := 0
			proof := freezeCaptureProcess(t, privateDir(t), stdout.Bytes(), "exited", &zero)
			if e := proof.check(m, hash, testSource); e == nil {
				t.Fatal("unproved capture result accepted", name)
			}
		})
	}
	zero := int64(0)
	m.Collections[0].Count = &zero
	if e := fixtureCaptureEvidence(t, m, hash).check(m, hash, testSource); e != nil {
		t.Fatal("a genuinely observed zero count was treated as missing", e)
	}
}

func TestCaptureReceiptRequiresExactlyTwoOriginalCollectionCounts(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	var stdout bytes.Buffer
	if deliverReceipt(fixtureCaptureReceipt(m, hash), &stdout) != 0 {
		t.Fatal("actual successful capture encoder failed")
	}
	original := stdout.Bytes()
	counts, e := json.Marshal([2]*int64{m.Collections[0].Count, m.Collections[1].Count})
	if e != nil {
		t.Fatal(e)
	}
	for _, extra := range []string{"valid", "0", "null", "{}", "[0]", "0,0"} {
		t.Run(extra, func(t *testing.T) {
			raw := original
			if extra != "valid" {
				field := []byte(`"exact_fixed_collection_counts":`)
				mutated := append(append([]byte(nil), counts[:len(counts)-1]...), []byte(","+extra+"]")...)
				raw = bytes.Replace(original, append(append([]byte(nil), field...), counts...), append(field, mutated...), 1)
				if bytes.Equal(raw, original) {
					t.Fatal("original count field was not mutated")
				}
			}
			zero := 0
			proof := freezeCaptureProcess(t, privateDir(t), raw, "exited", &zero)
			e := proof.check(m, hash, testSource)
			if extra == "valid" && e != nil || extra != "valid" && (e == nil || e.Error() != "capture_success_receipt_rejected") {
				t.Fatal("capture stdout accepted a count array the original tool cannot emit", extra, e)
			}
		})
	}
}

func TestCaptureReceiptRejectsFieldsOmittedFromSuccessfulStdout(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	var stdout bytes.Buffer
	if exit := deliverReceipt(fixtureCaptureReceipt(m, hash), &stdout); exit != 0 {
		t.Fatal("successful capture fixture did not produce original CLI stdout", exit)
	}
	original := append([]byte(nil), stdout.Bytes()...)
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(original, &fields); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"release_error_category", "restored_verification"} {
		if _, present := fields[name]; present {
			t.Fatal("successful capture encoder emitted an inapplicable field", name)
		}
	}
	for _, input := range []struct {
		name, member string
	}{
		{"valid_absence", ""},
		{"empty_release", `"release_error_category":""`},
		{"nonempty_release", `"release_error_category":"owned_resource_release_unproven"`},
		{"null_release", `"release_error_category":null`},
		{"null_restored", `"restored_verification":null`},
		{"object_restored", `"restored_verification":{}`},
		{"array_restored", `"restored_verification":[]`},
		{"string_restored", `"restored_verification":""`},
		{"number_restored", `"restored_verification":0`},
		{"bool_restored", `"restored_verification":false`},
	} {
		t.Run(input.name, func(t *testing.T) {
			raw := append([]byte(nil), original...)
			if input.member != "" {
				raw = bytes.TrimSpace(raw)
				raw = append(raw[:len(raw)-1], []byte(","+input.member+"}")...)
			}
			zero := 0
			proof := freezeCaptureProcess(t, privateDir(t), raw, "exited", &zero)
			e := proof.check(m, hash, testSource)
			if input.member == "" && e != nil || input.member != "" && (e == nil || e.Error() != "capture_success_receipt_rejected") {
				t.Fatal("capture evidence accepted fields its successful CLI encoder omits", input.name, e)
			}
		})
	}
}

func TestCaptureEvidenceUsesPrivateOriginalFilesAndBothRawHashes(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	for _, target := range []string{"executor", "stdout"} {
		for _, fault := range []string{"raw_hash", "missing", "public_mode", "symlink", "hardlink"} {
			t.Run(target+"/"+fault, func(t *testing.T) {
				proof := fixtureCaptureEvidence(t, m, hash)
				path := proof.path
				if target == "stdout" {
					raw, _ := os.ReadFile(proof.path)
					var execution captureExecution
					if strictDecode(raw, &execution) != nil {
						t.Fatal("fixture executor document")
					}
					path = execution.ReceiptFile
				}
				var e error
				switch fault {
				case "raw_hash":
					if target == "executor" {
						proof.hash = strings.Repeat("d", 64)
					} else {
						// Same length, permissions and valid JSON; original raw SHA
						// must still reject changed stdout whitespace bytes.
						raw, _ := os.ReadFile(path)
						raw[len(raw)-1] = ' '
						e = os.WriteFile(path, raw, 0600)
					}
				case "missing":
					e = os.Remove(path)
				case "public_mode":
					e = os.Chmod(path, 0644)
				case "symlink":
					moved := path + ".retained"
					if e = os.Rename(path, moved); e == nil {
						e = os.Symlink(moved, path)
					}
				case "hardlink":
					e = os.Link(path, path+".linked")
				}
				if e != nil {
					t.Fatal("owned evidence fault unavailable", e)
				}
				if proof.check(m, hash, testSource) == nil {
					t.Fatal("unbound or ineligible capture evidence accepted", target, fault)
				}
			})
		}
	}
}

func TestCaptureReceiptRejectsMissingNullDuplicateAndUnknownMembers(t *testing.T) {
	m := fixtureMaterial(t)
	hash := strings.Repeat("c", 64)
	for _, field := range captureReceiptFields {
		for _, missing := range []bool{false, true} {
			kind := "null"
			if missing {
				kind = "missing"
			}
			t.Run(kind+"/"+field, func(t *testing.T) {
				raw, _ := json.Marshal(fixtureCaptureReceipt(m, hash))
				var fields map[string]any
				if json.Unmarshal(raw, &fields) != nil {
					t.Fatal("fixture stdout")
				}
				fields[field] = nil
				if missing {
					delete(fields, field)
				}
				raw, _ = json.Marshal(fields)
				zero := 0
				proof := freezeCaptureProcess(t, privateDir(t), raw, "exited", &zero)
				if proof.check(m, hash, testSource) == nil {
					t.Fatal("incomplete original stdout accepted", field)
				}
			})
		}
	}
	for _, fault := range []string{"duplicate", "unknown", "case_alias", "null_optional_release", "null_optional_restored", "trailing_document", "oversize"} {
		t.Run(fault, func(t *testing.T) {
			raw, _ := json.Marshal(fixtureCaptureReceipt(m, hash))
			switch fault {
			case "duplicate":
				raw = append(raw[:len(raw)-1], []byte(`,"ledger_commit_outcome_unknown":false}`)...)
			case "unknown":
				raw = append(raw[:len(raw)-1], []byte(`,"snapshot_completed":true}`)...)
			case "case_alias":
				raw = bytes.Replace(raw, []byte(`"complete":true`), []byte(`"complete":false`), 1)
				raw = append(raw[:len(raw)-1], []byte(`,"Complete":true}`)...)
			case "null_optional_release":
				raw = append(raw[:len(raw)-1], []byte(`,"release_error_category":null}`)...)
			case "null_optional_restored":
				raw = append(raw[:len(raw)-1], []byte(`,"restored_verification":null}`)...)
			case "trailing_document":
				raw = append(raw, []byte(` {}`)...)
			case "oversize":
				raw = append(raw, bytes.Repeat([]byte(" "), maxCaptureReceiptBytes)...)
			}
			zero := 0
			proof := freezeCaptureProcess(t, privateDir(t), raw, "exited", &zero)
			if proof.check(m, hash, testSource) == nil {
				t.Fatal("ambiguous or unbounded original stdout accepted", fault)
			}
		})
	}
}

func TestCaptureEvidenceIsMandatoryAndRejectedOnNonconsumingActions(t *testing.T) {
	base := []string{"--mysql-database=qs_owned", "--mongo-database=qs_owned"}
	proof := captureEvidenceInput{"/private/executor.json", strings.Repeat("a", 64)}
	for _, action := range []string{"inspect", "capture", "verify-target", "restore", "verify-restored"} {
		args := append(append([]string{}, base...), "--action="+action)
		if action == "capture" {
			args = append(args, "--capture-file=/private/first.json")
		}
		if action != "capture" && action != "inspect" {
			args = append(args, "--material-file=/private/first.json", "--material-sha256="+strings.Repeat("b", 64))
			if action != "verify-target" {
				args = append(args, "--approval-file=/private/approval.json", "--approval-sha256="+strings.Repeat("c", 64))
			}
			if _, e := parseArgs(args); e == nil {
				t.Fatal("consumer accepted no independent capture evidence", action)
			}
		}
		for _, flags := range [][]string{captureEvidenceArgs(proof), {"--capture-evidence-file=" + proof.path}, {"--capture-evidence-sha256=" + proof.hash}} {
			_, e := parseArgs(append(append([]string{}, args...), flags...))
			want := action != "inspect" && action != "capture" && len(flags) == 2
			if want && e != nil || !want && e == nil {
				t.Fatal("capture evidence action/pair boundary failed", action, flags, e)
			}
		}
	}
}

func TestNonconsumersRejectExplicitEmptyCaptureEvidenceBeforeEnvironment(t *testing.T) {
	for _, action := range []string{"inspect", "capture"} {
		t.Run(action, func(t *testing.T) {
			for _, input := range []struct {
				name  string
				flags []string
			}{
				{"empty_file", []string{"--capture-evidence-file="}},
				{"empty_hash", []string{"--capture-evidence-sha256="}},
				{"empty_pair", []string{"--capture-evidence-file=", "--capture-evidence-sha256="}},
				{"empty_file_argument", []string{"--capture-evidence-file", ""}},
				{"empty_hash_argument", []string{"--capture-evidence-sha256", ""}},
			} {
				t.Run(input.name, func(t *testing.T) {
					args := []string{"--action=" + action, "--mysql-database=qs_owned", "--mongo-database=qs_owned"}
					if action == "capture" {
						args = append(args, "--capture-file="+filepath.Join(privateDir(t), "unused-material.json"))
					}
					args = append(args, input.flags...)
					if _, e := parseArgs(args); e == nil {
						t.Error("nonconsumer accepted an explicitly supplied empty capture evidence flag")
					}
					var stdout bytes.Buffer
					reads := 0
					exit := serve(context.Background(), args, func(string) string { reads++; return "" }, testSource, &stdout)
					var r receipt
					if strictDecode(stdout.Bytes(), &r) != nil || exit == 0 || r.Complete || reads != 0 {
						t.Errorf("explicit empty proof input crossed the pre-environment action boundary: exit=%d reads=%d receipt=%+v", exit, reads, r)
					}
				})
			}
		})
	}
}

func TestCaptureEvidenceIsRecheckedBeforeClaimAndRestoredVerification(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	proof := fixtureCaptureEvidence(t, m, mh)
	if e := rereadApproval(m, a, mp, mh, ap, ah, testSource, proof); e != nil {
		t.Fatal(e)
	}
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	if e := checkRestoredAuthorization(m, a, mp, mh, ap, ah, testSource, proof); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(proof.path)
	var execution captureExecution
	if strictDecode(raw, &execution) != nil || os.WriteFile(execution.ReceiptFile, []byte(`{"complete":true}`), 0600) != nil {
		t.Fatal("could not mutate owned original stdout")
	}
	journal, _ := recoveryJournalPath(m, mh)
	before, _ := os.ReadFile(journal)
	if rereadApproval(m, a, mp, mh, ap, ah, testSource, proof) == nil || checkRestoredAuthorization(m, a, mp, mh, ap, ah, testSource, proof) == nil {
		t.Fatal("external capture proof was not reread at original mutable/verification boundaries")
	}
	after, _ := os.ReadFile(journal)
	if !bytes.Equal(before, after) {
		t.Fatal("evidence recheck rewrote consumed authorization")
	}
}

func TestCaptureEvidenceAllowsOriginalSuccessfulReceiptBeforeConnections(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	proof := fixtureCaptureEvidence(t, m, mh)
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	for _, action := range []string{"verify-target", "restore", "verify-restored"} {
		t.Run(action, func(t *testing.T) {
			args := append([]string{"--action=" + action, "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + mp, "--material-sha256=" + mh}, captureEvidenceArgs(proof)...)
			if action != "verify-target" {
				args = append(args, "--approval-file="+ap, "--approval-sha256="+ah)
			}
			reads := 0
			r := run(context.Background(), args, func(string) string { reads++; return "" }, testSource, nil)
			if reads == 0 || r.Error != "selected_connection_input_rejected" || r.Complete {
				t.Fatal("original successful receipt did not reach the independent connection boundary", r, reads)
			}
		})
	}
}

func TestCaptureEvidenceRejectsReencodedOriginalStdout(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	var encoded bytes.Buffer
	if deliverReceipt(fixtureCaptureReceipt(m, mh), &encoded) != 0 {
		t.Fatal("actual successful receipt encoder failed")
	}
	original := encoded.Bytes()
	var reordered map[string]json.RawMessage
	if json.Unmarshal(original, &reordered) != nil {
		t.Fatal("original receipt unavailable")
	}
	reorderedRaw, e := json.Marshal(reordered)
	if e != nil {
		t.Fatal(e)
	}
	var indented bytes.Buffer
	if json.Indent(&indented, original, "", "  ") != nil {
		t.Fatal("independent JSON transformation failed")
	}
	for _, candidate := range []struct {
		name string
		raw  []byte
	}{
		{"original", original},
		{"reordered_members", append(reorderedRaw, '\n')},
		{"indented", indented.Bytes()},
		{"escaped_key", bytes.Replace(original, []byte(`"action"`), []byte(`"act\u0069on"`), 1)},
		{"escaped_value", bytes.Replace(original, []byte(`"capture"`), []byte(`"capt\u0075re"`), 1)},
		{"leading_whitespace", append([]byte(" "), original...)},
		{"missing_final_newline", bytes.TrimSuffix(original, []byte("\n"))},
		{"CRLF", append(append([]byte(nil), original[:len(original)-1]...), '\r', '\n')},
		{"extra_final_newline", append(append([]byte(nil), original...), '\n')},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			if !json.Valid(candidate.raw) || candidate.name != "original" && bytes.Equal(candidate.raw, original) {
				t.Fatal("candidate is not the intended distinct valid JSON")
			}
			zero := 0
			proof := freezeCaptureProcess(t, privateDir(t), candidate.raw, "exited", &zero)
			for _, action := range []string{"verify-target", "restore", "verify-restored"} {
				t.Run(action, func(t *testing.T) {
					args := append([]string{"--action=" + action, "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + mp, "--material-sha256=" + mh}, captureEvidenceArgs(proof)...)
					if action != "verify-target" {
						args = append(args, "--approval-file="+ap, "--approval-sha256="+ah)
					}
					reads := 0
					r := run(context.Background(), args, func(string) string { reads++; return "" }, testSource, nil)
					if candidate.name == "original" {
						if reads == 0 || r.Error != "selected_connection_input_rejected" || r.Complete {
							t.Fatal("actual encoder output did not reach the connection boundary", reads, r)
						}
					} else if reads != 0 || r.Error != "capture_success_receipt_rejected" || r.Complete || r.SQLConnectionClosed != nil || r.SQLPoolClosed != nil || r.MongoClosed != nil {
						t.Fatal("reencoded output crossed the pre-environment evidence boundary", reads, r)
					}
				})
			}
		})
	}
}
