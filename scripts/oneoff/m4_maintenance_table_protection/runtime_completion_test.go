package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompletionArgumentsHaveIndependentAuthorizationBoundary(t *testing.T) {
	dir := privateFixture(t)
	h := fingerprint([]byte("independently frozen input"))
	args := append(cliArgs("complete-first-runtime"), "--material-file="+filepath.Join(dir, "capture.json"), "--material-sha256="+h, "--capture-evidence-file="+filepath.Join(dir, "capture-execution.json"), "--capture-evidence-sha256="+h, "--runtime-intent-file="+filepath.Join(dir, "intent.json"), "--runtime-intent-sha256="+h, "--completion-approval-file="+filepath.Join(dir, "completion.json"), "--completion-approval-sha256="+h)
	if _, e := parseArgs(args); e != nil {
		t.Fatal("independently bound completion input unavailable", e)
	}
}

func completionFixture(t *testing.T, mode string, age time.Duration) (runtimeCompletionInputs, arguments, runtimeBoundaryObservation) {
	t.Helper()
	now := time.Now().UTC()
	then := now.Add(-age)
	m := originalMaterial()
	m.CapturedAt = then.Add(-time.Minute).Format(time.RFC3339Nano)
	m.CapturePath = filepath.Join(privateFixture(t), "capture.json")
	raw, _ := json.Marshal(m)
	if e := writeExclusive(m.CapturePath, raw); e != nil {
		t.Fatal(e)
	}
	hash := fingerprint(raw)
	old := approval{Format: 1, Kind: "m4_d_pre_start_maintenance_table_restore", Operation: "123-1", ToolSource: m.ToolSource, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, MaterialHash: hash, FreezeHash: fingerprint([]byte("old freeze")), ResponsibilityHash: fingerprint([]byte("old owners")), Binding: m.Binding, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: derivedRuntimeMarker(m), WindowStarted: then.Add(-2 * time.Minute).Format(time.RFC3339Nano), RecoveryDecision: then.Add(-time.Second).Format(time.RFC3339Nano), Deadline: then.Add(30 * time.Minute).Format(time.RFC3339Nano), Expires: then.Add(time.Hour).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	intent := fixtureRuntimeIntent(m, hash, old)
	intent.Mode, intent.ApplyNotDispatched = mode, mode == "cancel102"
	intent.Decision = then.Format(time.RFC3339Nano)
	intent.Deadline = then.Add(10 * time.Minute).Format(time.RFC3339Nano)
	a := arguments{action: "complete-first-runtime", material: m.CapturePath, materialHash: hash, runtimeIntent: filepath.Join(filepath.Dir(m.CapturePath), "intent.json"), binding: m.Binding, timeout: time.Minute, completionApproval: filepath.Join(filepath.Dir(m.CapturePath), "completion.json")}
	if mode == "restored102" {
		intent.Deadline = old.Deadline
		a.approval = filepath.Join(filepath.Dir(m.CapturePath), "old-restore.json")
		raw, _ = json.Marshal(old)
		a.approvalHash = fingerprint(raw)
		if e := writeExclusive(a.approval, raw); e != nil {
			t.Fatal(e)
		}
		if e := consumeApproval(m, old, a.approvalHash, ledgerRow{103, false}, []bool{true, true, true, true, true}); e != nil {
			t.Fatal(e)
		}
	}
	raw, _ = json.Marshal(intent)
	a.runtimeIntentHash = fingerprint(raw)
	if e := writeExclusive(a.runtimeIntent, raw); e != nil {
		t.Fatal(e)
	}
	proof := unitCaptureEvidenceArgs(t, m, hash)
	a.captureEvidence = strings.TrimPrefix(proof[0], "--capture-evidence-file=")
	a.captureEvidenceHash = strings.TrimPrefix(proof[1], "--capture-evidence-sha256=")
	rec, raw, e := nextRuntimeBoundary(m, intent, a, m.ToolSource, then.Add(time.Millisecond))
	if e != nil {
		t.Fatal(e)
	}
	o := runtimeBoundaryObservation{State: runtimeBoundaryState{rec.Generation, fingerprint(raw)}, Record: &rec, Raw: raw}
	current := runtimeCompletionApproval{Format: 1, Kind: "m4_d_complete_committed_runtime_intent", Operation: "8000-1", ToolSource: testTool, OriginalToolSource: m.ToolSource, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, Binding: m.Binding, MaterialHash: hash, CaptureEvidenceHash: a.captureEvidenceHash, IntentHash: a.runtimeIntentHash, RestoreApprovalHash: a.approvalHash, Boundary: o.State, RecordKind: rec.Kind, Mode: mode, SQLVersion: rec.SQLVersion, FactsHash: completionFactsHash(expectedCompletionFacts(m, mode)), Marker: derivedRuntimeMarker(m), MarkerState: "absent", MarkerHash: fingerprint(deterministicCompletionMarker(o, m.Binding)), FreezeHash: fingerprint([]byte("fresh freeze")), ResponsibilityHash: fingerprint([]byte("fresh owners")), Decision: now.Add(time.Millisecond).Format(time.RFC3339Nano), Expires: now.Add(5 * time.Minute).Format(time.RFC3339Nano), DataOwner: "system (10001)", RecoveryOwner: "system (10001)", RuntimeFrozen: true, RuntimeNotStarted: true, AllowCompletion: true}
	// Keep the current decision after the original record while avoiding a future
	// decision when this fixture is immediately consumed by the real CLI reader.
	current.Decision = time.Now().UTC().Format(time.RFC3339Nano)
	if mustTime(current.Decision).Before(mustTime(rec.Created)) {
		time.Sleep(2 * time.Millisecond)
		current.Decision = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, _ = json.Marshal(current)
	a.completionApprovalHash = fingerprint(raw)
	if e := writeExclusive(a.completionApproval, raw); e != nil {
		t.Fatal(e)
	}
	var restored *approval
	if mode == "restored102" {
		restored = &old
	}
	return runtimeCompletionInputs{current, m, intent, restored}, a, o
}

func completionCLI(a arguments) []string {
	args := append(cliArgs("complete-first-runtime"), "--material-file="+a.material, "--material-sha256="+a.materialHash, "--capture-evidence-file="+a.captureEvidence, "--capture-evidence-sha256="+a.captureEvidenceHash, "--runtime-intent-file="+a.runtimeIntent, "--runtime-intent-sha256="+a.runtimeIntentHash, "--completion-approval-file="+a.completionApproval, "--completion-approval-sha256="+a.completionApprovalHash)
	if a.approval != "" {
		args = append(args, "--approval-file="+a.approval, "--approval-sha256="+a.approvalHash)
	}
	return args
}

func writeCompletionApproval(t *testing.T, v runtimeCompletionApproval, a *arguments) {
	t.Helper()
	raw, _ := json.Marshal(v)
	if e := os.WriteFile(a.completionApproval, raw, 0600); e != nil {
		t.Fatal(e)
	}
	a.completionApprovalHash = fingerprint(raw)
}

func TestCompletionCurrentApprovalQualifiesOriginalExpiredIntentOnlyForConfirmation(t *testing.T) {
	for _, mode := range []string{"target103", "cancel102", "restored102"} {
		t.Run(mode, func(t *testing.T) {
			age := 2 * time.Hour
			if mode == "restored102" {
				age = 0
			}
			v, a, o := completionFixture(t, mode, age)
			loaded, e := readRuntimeCompletionInputs(a, testTool, time.Now())
			if e != nil || !reflect.DeepEqual(v, loaded) {
				t.Fatal("valid explicit current authorization rejected", e)
			}
			if e := loaded.matchCommitted(o, a); e != nil {
				t.Fatal(e)
			}
			if age > 0 && validateRuntimeIntent(v.intent, v.original, a.materialHash, testTool, nil, time.Now()) == nil {
				t.Fatal("expired original regained live mark authority")
			}
			gets := 0
			r := run(t.Context(), completionCLI(a), func(string) string { gets++; return "" }, testTool)
			if r.Complete || r.Error != "selected_connection_input_rejected" || gets != 2 || r.RuntimeCompletion != nil || r.ApprovalConsumed || r.MarkerWritten {
				t.Fatal("qualified completion did not reach exact ENV sentinel without authority", gets, r)
			}
		})
	}
}

func TestCompletionApprovalAndInputsRejectBeforeEnvironment(t *testing.T) {
	cases := map[string]func(*runtimeCompletionApproval, *arguments){
		"current_source":  func(v *runtimeCompletionApproval, _ *arguments) { v.ToolSource = strings.Repeat("b", 40) },
		"original_source": func(v *runtimeCompletionApproval, _ *arguments) { v.OriginalToolSource = strings.Repeat("b", 40) },
		"resources":       func(v *runtimeCompletionApproval, _ *arguments) { v.DResources = fingerprint([]byte("foreign")) },
		"material":        func(v *runtimeCompletionApproval, _ *arguments) { v.MaterialHash = fingerprint([]byte("foreign")) },
		"capture_execution": func(v *runtimeCompletionApproval, _ *arguments) {
			v.CaptureEvidenceHash = fingerprint([]byte("foreign"))
		},
		"intent":             func(v *runtimeCompletionApproval, _ *arguments) { v.IntentHash = fingerprint([]byte("foreign")) },
		"binding":            func(v *runtimeCompletionApproval, _ *arguments) { v.Binding.MongoUUID = fingerprint([]byte("foreign")) },
		"record_kind":        func(v *runtimeCompletionApproval, _ *arguments) { v.RecordKind = "foreign" },
		"initial_generation": func(v *runtimeCompletionApproval, a *arguments) { v.Boundary = initialRuntimeBoundary(a.binding) },
		"mode_version":       func(v *runtimeCompletionApproval, _ *arguments) { v.SQLVersion = 102 },
		"facts":              func(v *runtimeCompletionApproval, _ *arguments) { v.FactsHash = fingerprint([]byte("foreign")) },
		"marker_path": func(v *runtimeCompletionApproval, _ *arguments) {
			v.Marker = filepath.Join(filepath.Dir(v.Marker), "other")
		},
		"marker_state": func(v *runtimeCompletionApproval, _ *arguments) { v.MarkerState = "repair_partial" },
		"freeze":       func(v *runtimeCompletionApproval, _ *arguments) { v.RuntimeFrozen = false },
		"started":      func(v *runtimeCompletionApproval, _ *arguments) { v.RuntimeNotStarted = false },
		"permission":   func(v *runtimeCompletionApproval, _ *arguments) { v.AllowCompletion = false },
		"owner":        func(v *runtimeCompletionApproval, _ *arguments) { v.DataOwner = "unbound" },
		"expired": func(v *runtimeCompletionApproval, _ *arguments) {
			v.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
		},
		"extension": func(v *runtimeCompletionApproval, _ *arguments) {
			v.Expires = mustTime(v.Decision).Add(maxAction + time.Second).Format(time.RFC3339Nano)
		},
		"future_decision": func(v *runtimeCompletionApproval, _ *arguments) {
			v.Decision = time.Now().Add(time.Minute).Format(time.RFC3339Nano)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v, a, _ := completionFixture(t, "target103", 0)
			change(&v.approval, &a)
			writeCompletionApproval(t, v.approval, &a)
			r := run(t.Context(), completionCLI(a), func(string) string { t.Fatal("invalid current approval reached ENV"); return "" }, testTool)
			if r.Complete || r.RuntimeCompletion != nil || r.ApprovalConsumed || r.MarkerWritten {
				t.Fatal(r)
			}
		})
	}
}

func TestCompletionApprovalRequiresLiteralCompletePrivateDocument(t *testing.T) {
	cases := map[string]func(map[string]any){
		"alias": func(v map[string]any) {
			v["Allow_Committed_Intent_Confirmation"] = v["allow_committed_intent_confirmation"]
			delete(v, "allow_committed_intent_confirmation")
		},
		"null_false_risk":         func(v map[string]any) { v["first_runtime_command_not_issued"] = nil },
		"missing_false_risk":      func(v map[string]any) { delete(v, "first_runtime_command_not_issued") },
		"unknown":                 func(v map[string]any) { v["allow_start"] = true },
		"null_empty_restore_hash": func(v map[string]any) { v["original_consumed_restore_approval_sha256"] = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			_, a, _ := completionFixture(t, "target103", 0)
			raw, _ := os.ReadFile(a.completionApproval)
			var tree map[string]any
			_ = json.Unmarshal(raw, &tree)
			change(tree)
			raw, _ = json.Marshal(tree)
			if e := os.WriteFile(a.completionApproval, raw, 0600); e != nil {
				t.Fatal(e)
			}
			a.completionApprovalHash = fingerprint(raw)
			r := run(t.Context(), completionCLI(a), func(string) string { t.Fatal("nonliteral approval reached ENV"); return "" }, testTool)
			if r.Error != "runtime_completion_approval_rejected" {
				t.Fatal(r)
			}
		})
	}
	for _, name := range []string{"duplicate", "hash", "public", "hardlink", "oversize"} {
		t.Run(name, func(t *testing.T) {
			_, a, _ := completionFixture(t, "target103", 0)
			raw, _ := os.ReadFile(a.completionApproval)
			switch name {
			case "duplicate":
				raw = append(raw[:len(raw)-1], []byte(`,"format_version":1}`)...)
			case "hash":
				a.completionApprovalHash = fingerprint([]byte("wrong"))
			case "public":
				if e := os.Chmod(a.completionApproval, 0644); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(a.completionApproval, a.completionApproval+".link"); e != nil {
					t.Fatal(e)
				}
			case "oversize":
				raw = []byte(strings.Repeat(" ", maxRuntimeIntentBytes+1))
			}
			if name == "duplicate" || name == "oversize" {
				if e := os.WriteFile(a.completionApproval, raw, 0600); e != nil {
					t.Fatal(e)
				}
				a.completionApprovalHash = fingerprint(raw)
			}
			if _, e := readRuntimeCompletionInputs(a, testTool, time.Now()); e == nil {
				t.Fatal("unqualified private approval accepted")
			}
		})
	}
}

func TestCompletionRequiresExactCommittedGenerationIntentAndOriginalBytes(t *testing.T) {
	cases := map[string]func(*runtimeBoundaryObservation){
		"no_row":           func(o *runtimeBoundaryObservation) { o.Record = nil },
		"next_generation":  func(o *runtimeBoundaryObservation) { o.Record.Generation++; o.State.Generation++ },
		"raw_hash":         func(o *runtimeBoundaryObservation) { o.Raw = append(o.Raw, ' ') },
		"foreign_material": func(o *runtimeBoundaryObservation) { o.Record.MaterialHash = fingerprint([]byte("foreign")) },
		"foreign_intent":   func(o *runtimeBoundaryObservation) { o.Record.IntentHash = fingerprint([]byte("foreign")) },
		"foreign_source":   func(o *runtimeBoundaryObservation) { o.Record.ToolSource = strings.Repeat("b", 40) },
		"foreign_previous": func(o *runtimeBoundaryObservation) { o.Record.Previous = fingerprint([]byte("foreign")) },
		"future_created": func(o *runtimeBoundaryObservation) {
			o.Record.Created = time.Now().Add(time.Minute).Format(time.RFC3339Nano)
		},
		"after_original_deadline": func(o *runtimeBoundaryObservation) {
			o.Record.Created = time.Now().Add(11 * time.Minute).Format(time.RFC3339Nano)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v, a, o := completionFixture(t, "target103", 0)
			change(&o)
			if v.matchCommitted(o, a) == nil {
				t.Fatal("foreign or unknown committed fact accepted")
			}
		})
	}
	// Model an actual different canonical committed record rather than relying on
	// stale raw/hash to reject every changed field.
	for _, name := range []string{"intent", "previous", "source", "mode", "late"} {
		t.Run("canonical_"+name, func(t *testing.T) {
			v, a, o := completionFixture(t, "target103", 0)
			switch name {
			case "intent":
				o.Record.IntentHash = fingerprint([]byte("different intent"))
			case "previous":
				o.Record.Previous = fingerprint([]byte("different predecessor"))
			case "source":
				o.Record.ToolSource = strings.Repeat("b", 40)
			case "mode":
				o.Record.Mode = "cancel102"
				o.Record.SQLVersion = 102
			case "late":
				o.Record.Created = mustTime(v.intent.Deadline).Add(time.Second).Format(time.RFC3339Nano)
				v.approval.Decision = mustTime(o.Record.Created).Add(time.Second).Format(time.RFC3339Nano)
			}
			o.Raw, _ = json.Marshal(o.Record)
			o.State.Fingerprint = fingerprint(o.Raw)
			v.approval.Boundary = o.State
			if v.matchCommitted(o, a) == nil {
				t.Fatal("canonical same-hash-authorized foreign record crossed original history")
			}
		})
	}
}

func TestCompletionDeterministicMarkerAndReadOnlyExistingConfirmation(t *testing.T) {
	for _, oldFormat := range []bool{false, true} {
		t.Run(fmt.Sprint(oldFormat), func(t *testing.T) {
			v, a, o := completionFixture(t, "target103", 0)
			raw := deterministicCompletionMarker(o, a.binding)
			if oldFormat {
				key, _ := runtimeGateKey(a.binding)
				raw, _ = json.Marshal(originalRuntimeMarker{"m4_d_first_runtime_boundary", v.original.ToolSource, dSource, dResources, a.materialHash, a.runtimeIntentHash, v.intent.Mode, key, o.Record.Created, 103})
			}
			v.approval.MarkerHash = fingerprint(raw)
			if e := writeExclusive(v.approval.Marker, raw); e != nil {
				t.Fatal(e)
			}
			if v.checkMarkerBefore(o, a) == nil {
				t.Fatal("absent approval silently became existing confirmation")
			}
			v.approval.MarkerState = "same_existing"
			for range 2 {
				if e := v.checkMarkerBefore(o, a); e != nil {
					t.Fatal("same existing was not idempotent", e)
				}
			}
			got, e := os.ReadFile(v.approval.Marker)
			if e != nil || !bytes.Equal(got, raw) {
				t.Fatal("existing bytes were rewritten")
			}
			if validateRuntimeIntent(v.intent, v.original, a.materialHash, testTool, nil, time.Now()) == nil {
				t.Fatal("new completion marker reopened old mark")
			}
			if markerAbsent(v.approval.Marker) == nil {
				t.Fatal("old recovery reader ignored new boundary")
			}
		})
	}
	v, a, o := completionFixture(t, "target103", 0)
	first := deterministicCompletionMarker(o, a.binding)
	v.approval.Operation = "8001-1"
	v.approval.Decision = time.Now().Add(time.Second).Format(time.RFC3339Nano)
	v.approval.ToolSource = strings.Repeat("b", 40)
	if !bytes.Equal(first, deterministicCompletionMarker(o, a.binding)) {
		t.Fatal("current authorization changed old intent marker bytes")
	}
	for _, raw := range [][]byte{[]byte("partial"), []byte(`{"kind":"foreign"}`), append(first, ' ')} {
		if e := os.WriteFile(v.approval.Marker, raw, 0600); e != nil {
			t.Fatal(e)
		}
		v.approval.MarkerHash = fingerprint(raw)
		v.approval.MarkerState = "same_existing"
		if v.verifyMarker(o, a) == nil {
			t.Fatal("foreign/partial or noncanonical completion marker accepted")
		}
	}
}

func TestCompletionFactsProjectionProtectsCompleteScopeWithoutChangingOriginal(t *testing.T) {
	v, _, _ := completionFixture(t, "target103", 0)
	raw, _ := json.Marshal(v.original)
	expected := expectedCompletionFacts(v.original, "target103")
	for _, field := range []string{"retained", "mongo", "ledger", "historical", "session", "identity"} {
		t.Run(field, func(t *testing.T) {
			copyRaw, _ := json.Marshal(expected)
			var changed runtimeCompletionFacts
			_ = json.Unmarshal(copyRaw, &changed)
			switch field {
			case "retained":
				changed.Retained[0].EOF = false
			case "mongo":
				changed.Mongo = append(changed.Mongo, 0)
			case "ledger":
				changed.Ledger[0].Dirty = true
			case "historical":
				changed.Tables[0] = v.original.Tables[0]
			case "session":
				changed.Zone = "foreign"
			case "identity":
				changed.Binding.SQLIdentity = fingerprint([]byte("foreign"))
			}
			if completionFactsHash(changed) == completionFactsHash(expected) {
				t.Fatal("complete fact drift hidden")
			}
		})
	}
	after, _ := json.Marshal(v.original)
	if !bytes.Equal(raw, after) || len(v.original.Ledger) != 1 || v.original.Ledger[0].Version != 102 {
		t.Fatal("target projection rewrote original material")
	}
}

func TestCompletionHistoricalConsumedScopeDoesNotCreateOrRenewRecoveryState(t *testing.T) {
	v, a, o := completionFixture(t, "restored102", 0)
	if e := v.matchCommitted(o, a); e != nil {
		t.Fatal(e)
	}
	path, _ := recoveryJournalPath(v.original, a.materialHash)
	before, _ := os.ReadFile(path)
	if e := v.checkConsumedHistory(a, mustTime(o.Record.Created)); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only historical qualification changed journal")
	}
	_, claim, _ := recoveryPaths(v.original, a.materialHash, v.restored.Operation)
	if e := os.Remove(claim); e != nil {
		t.Fatal(e)
	}
	if v.checkConsumedHistory(a, mustTime(o.Record.Created)) == nil {
		t.Fatal("missing original consumed claim was recreated")
	}
	if _, e := os.Stat(claim); !os.IsNotExist(e) {
		t.Fatal("historical checker wrote a claim")
	}
}

func TestCompletionNewFlagsCannotLeakIntoOldActions(t *testing.T) {
	for _, action := range []string{"inspect", "capture", "restore", "verify-target", "verify-restored", "mark-first-runtime"} {
		for _, flag := range []string{"--completion-approval-file=", "--completion-approval-sha256="} {
			t.Run(action+flag, func(t *testing.T) {
				r := run(t.Context(), append(cliArgs(action), flag), func(string) string { t.Fatal("completion flag reached old ENV"); return "" }, testTool)
				if r.Error != "runtime_completion_approval_action_mismatch" {
					t.Fatal(r)
				}
			})
		}
	}
}

func TestCompletionTypedOutputCannotQualifyOriginalCaptureOrBusinessStart(t *testing.T) {
	m := originalMaterial()
	hash := fingerprint([]byte("material"))
	r := unitCaptureReceipt(m, hash)
	for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`0`), json.RawMessage(`{}`)} {
		raw, _ := json.Marshal(r)
		var members map[string]json.RawMessage
		_ = json.Unmarshal(raw, &members)
		members["runtime_completion"] = value
		raw, _ = json.Marshal(members)
		var got receipt
		if decodeCaptureDocument(raw, &got) == nil && validateCaptureReceipt(got, m, hash, testTool) == nil {
			t.Fatal("completion member became original successful capture", string(value))
		}
	}
	r = receipt{Action: "complete-first-runtime", Complete: true, ReadOnly: true, Error: "none", RuntimeCompletion: &runtimeCompletionReceipt{AuthorizationMode: completionAuthorizationMode, BusinessStartAuthorized: false}}
	if emitReceipt(r, badOutput{}) != 1 {
		t.Fatal("lost completion output was accepted")
	}
	raw, _ := json.Marshal(r)
	if bytes.Contains(raw, []byte("completion_approval_consumed")) || r.MarkerWritten || r.ApprovalConsumed || r.DatabaseBoundaryAdvanced {
		t.Fatal("confirmation created consumption or business authority")
	}
}

func TestCompletionCrossSourceIsExplicitAndDoesNotLoosenOriginalConsumers(t *testing.T) {
	v, a, o := completionFixture(t, "target103", 0)
	current := strings.Repeat("b", 40)
	v.approval.ToolSource = current
	writeCompletionApproval(t, v.approval, &a)
	gets := 0
	r := run(t.Context(), completionCLI(a), func(string) string { gets++; return "" }, current)
	if r.Error != "selected_connection_input_rejected" || gets != 2 {
		t.Fatal("exact explicit original source did not qualify", r)
	}
	if e := v.matchCommitted(o, a); e != nil {
		t.Fatal(e)
	}
	args := append(cliArgs("verify-target"), "--material-file="+a.material, "--material-sha256="+a.materialHash, "--capture-evidence-file="+a.captureEvidence, "--capture-evidence-sha256="+a.captureEvidenceHash)
	r = run(t.Context(), args, func(string) string { t.Fatal("old consumer borrowed completion's old source exception"); return "" }, current)
	if r.Error != "original_material_rejected" {
		t.Fatal(r)
	}
}

func TestCompletionRejectsFailedCaptureAndFixedInputChanges(t *testing.T) {
	v, a, o := completionFixture(t, "target103", 0)
	raw, _ := os.ReadFile(a.captureEvidence)
	var execution captureExecutionEvidence
	_ = json.Unmarshal(raw, &execution)
	exit := 1
	execution.ExitCode = &exit
	raw, _ = json.Marshal(execution)
	if e := os.WriteFile(a.captureEvidence, raw, 0600); e != nil {
		t.Fatal(e)
	}
	a.captureEvidenceHash = fingerprint(raw)
	v.approval.CaptureEvidenceHash = a.captureEvidenceHash
	writeCompletionApproval(t, v.approval, &a)
	r := run(t.Context(), completionCLI(a), func(string) string { t.Fatal("success stdout with nonzero actual capture exit reached ENV"); return "" }, testTool)
	if r.Error != "original_capture_execution_evidence_rejected" {
		t.Fatal(r)
	}
	if v.reread(a, testTool) == nil {
		t.Fatal("locked/terminal reread accepted changed capture proof")
	}
	// The positive terminal path also uses the frozen original receipt and the
	// current independent approval, never only the already-created marker.
	v, a, o = completionFixture(t, "target103", 0)
	if e := writeExclusive(v.approval.Marker, deterministicCompletionMarker(o, a.binding)); e != nil {
		t.Fatal(e)
	}
	rec := &runtimeCompletionReceipt{Boundary: o.State}
	if e := v.verifyTerminal(a, testTool, rec, o.Record); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(a.runtimeIntent, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if v.verifyTerminal(a, testTool, rec, o.Record) == nil {
		t.Fatal("terminal accepted changed original intent after marker creation")
	}
}

func TestCompletionCancelledBudgetCannotReachEnvironment(t *testing.T) {
	_, a, _ := completionFixture(t, "target103", 0)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := run(ctx, completionCLI(a), func(string) string { t.Fatal("expired completion reached ENV after its new file IO"); return "" }, testTool)
	if r.Complete || !r.BudgetExceeded || r.Error != "total_budget_exceeded" {
		t.Fatal(r)
	}
}
