package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureRuntimeIntent(m material, hash string, ap approval) runtimeIntent {
	now := time.Now().UTC()
	return runtimeIntent{Format: 1, Kind: "m4_d_first_runtime_intent", Operation: "125-1", ToolSource: m.ToolSource, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, MaterialHash: hash, CapturePath: m.CapturePath, Marker: derivedRuntimeMarker(m), Binding: m.Binding, Mode: "target103", FreezeHash: ap.FreezeHash, ResponsibilityHash: ap.ResponsibilityHash, WindowStarted: ap.WindowStarted, Decision: now.Format(time.RFC3339Nano), Deadline: now.Add(10 * time.Minute).Format(time.RFC3339Nano), Expires: ap.Expires, RuntimeFrozen: true, RuntimeNotStarted: true, AllowMarker: true}
}

func TestRuntimeIntentThreeModesRequireTheirOwnEvidence(t *testing.T) {
	m, a, hash, path, ah := recoveryInputs(t)
	v := fixtureRuntimeIntent(m, hash, a)
	if validateRuntimeIntent(v, m, hash, testTool, nil, time.Now()) != nil {
		t.Fatal("normal target intent rejected")
	}
	v.Mode, v.ApplyNotDispatched = "cancel102", true
	if validateRuntimeIntent(v, m, hash, testTool, nil, time.Now()) != nil {
		t.Fatal("independent before-apply cancellation rejected")
	}
	if consumeApproval(m, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) != nil {
		t.Fatal("fixture actual prior admission unavailable")
	}
	if validateRuntimeIntent(v, m, hash, testTool, nil, time.Now()) == nil {
		t.Fatal("cancellation silently borrowed a recovery journal")
	}
	v.Mode, v.ApplyNotDispatched, v.Deadline = "restored102", false, a.Deadline
	if checkRestoredAuthorization(m, a, m.CapturePath, hash, path, ah, testTool) != nil || validateRuntimeIntent(v, m, hash, testTool, &a, time.Now()) != nil {
		t.Fatal("same consumed recovery scope rejected")
	}
	v.Deadline = time.Now().Add(31 * time.Minute).Format(time.RFC3339Nano)
	if validateRuntimeIntent(v, m, hash, testTool, &a, time.Now()) == nil {
		t.Fatal("runtime intent reset the original restoration deadline")
	}
}

func TestRuntimeIntentRejectsReboundMarkerBudgetAndPermission(t *testing.T) {
	m, a, hash, _, _ := recoveryInputs(t)
	changes := map[string]func(*runtimeIntent){
		"marker":         func(v *runtimeIntent) { v.Marker = filepath.Join(filepath.Dir(v.Marker), "other") },
		"material":       func(v *runtimeIntent) { v.MaterialHash = fingerprint([]byte("other")) },
		"permission":     func(v *runtimeIntent) { v.AllowMarker = false },
		"frozen":         func(v *runtimeIntent) { v.RuntimeFrozen = false },
		"mode":           func(v *runtimeIntent) { v.Mode = "legacy" },
		"normal_window":  func(v *runtimeIntent) { v.Deadline = time.Now().Add(40 * time.Minute).Format(time.RFC3339Nano) },
		"expired":        func(v *runtimeIntent) { v.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano) },
		"before_capture": func(v *runtimeIntent) { v.Decision = time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			v := fixtureRuntimeIntent(m, hash, a)
			change(&v)
			if validateRuntimeIntent(v, m, hash, testTool, nil, time.Now()) == nil {
				t.Fatal("rebound/expired/unapproved runtime author admitted")
			}
		})
	}
	a.Marker = filepath.Join(filepath.Dir(a.Marker), "legacy-free-marker")
	if validateApproval(a, m, hash, testTool, time.Now()) == nil {
		t.Fatal("legacy restore approval escaped derived first marker")
	}
}

func TestRuntimeIntentStrictRawSHAAndSizeFailBeforeEnvironment(t *testing.T) {
	m, a, hash, _, _ := recoveryInputs(t)
	v := fixtureRuntimeIntent(m, hash, a)
	good, _ := json.Marshal(v)
	for name, raw := range map[string][]byte{"wrong_sha": good, "unknown": append(good[:len(good)-1:len(good)-1], []byte(`,"extra":true}`)...), "duplicate": []byte(`{"format_version":1,"format_version":1}`), "overlimit": []byte(strings.Repeat(" ", maxRuntimeIntentBytes+1))} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(filepath.Dir(m.CapturePath), name+".json")
			if writeExclusive(p, raw) != nil {
				t.Fatal("private intent fixture unavailable")
			}
			ih := fingerprint(raw)
			if name == "wrong_sha" {
				ih = fingerprint([]byte("unrelated bytes"))
			}
			args := append(cliArgs("mark-first-runtime"), "--material-file="+m.CapturePath, "--material-sha256="+hash, "--runtime-intent-file="+p, "--runtime-intent-sha256="+ih)
			r := run(context.Background(), args, func(string) string { t.Fatal("ENV before strict intent rejection"); return "" }, testTool)
			if r.Complete || r.MarkerWritten || r.SQLConnectionClosed || r.RuntimeGateAcquired {
				t.Fatal("unapproved marker action progressed", r)
			}
		})
	}
}

func TestDerivedMarkerAlreadyPresentClosesAllRecoveryAuthors(t *testing.T) {
	m, a, hash, _, _ := recoveryInputs(t)
	v := fixtureRuntimeIntent(m, hash, a)
	raw := []byte("even an incomplete existing marker is never rewritten")
	if writeExclusive(v.Marker, raw) != nil {
		t.Fatal("marker fixture unavailable")
	}
	if validateRuntimeIntent(v, m, hash, testTool, nil, time.Now()) == nil || validateApproval(a, m, hash, testTool, time.Now()) == nil {
		t.Fatal("existing boundary admitted a later mutation or marker rewrite")
	}
	after, e := os.ReadFile(v.Marker)
	if e != nil || string(after) != string(raw) {
		t.Fatal("original first marker changed")
	}
}
