package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFirstBoundaryOnlyAcceptsFixed103DropPrefixes(t *testing.T) {
	for n := 0; n <= len(scope); n++ {
		missing := make([]bool, len(scope))
		for i := 0; i < n; i++ {
			missing[i] = true
		}
		for _, dirty := range []bool{false, true} {
			if validInitialRecoveryState(ledgerRow{103, dirty}, missing) != (dirty || n == len(scope)) {
				t.Fatal("incorrect first DROP-prefix admission", n, dirty)
			}
		}
	}
	for _, missing := range [][]bool{nil, {true}, {true, true, true, true}, {true, true, true, true, true, false}, {false, true, false, false, false}, {true, false, true, false, false}} {
		if validInitialRecoveryState(ledgerRow{103, true}, missing) {
			t.Fatal("unknown/non-prefix first state accepted", missing)
		}
	}
	if validInitialRecoveryState(ledgerRow{102, false}, []bool{true, true, true, true, true}) {
		t.Fatal("old ledger admitted as recovery")
	}
}

func TestFirstMissingSetCannotExpandOrChangeAfterDurableClaim(t *testing.T) {
	m, a, mh, _, ah := recoveryInputs(t)
	initial := ledgerRow{103, true}
	missing := []bool{true, true, false, false, false}
	if e := consumeApproval(m, a, ah, initial, missing); e != nil {
		t.Fatal(e)
	}
	state, e := readRecoveryBoundary(m, a, mh)
	if e != nil || state == nil || !sameMissing(state.InitialMissing, missing) || state.InitialLedger != initial {
		t.Fatal("initial missing set not fixed", e)
	}
	before, _ := json.Marshal(state)
	for _, changed := range [][]bool{{true, true, true, false, false}, {true, false, false, false, false}, {true, true, true, true, true}} {
		next := a
		next.Operation = "124-1"
		if consumeApproval(m, next, fingerprint([]byte("independent continuation")), initial, changed) == nil {
			t.Fatal("new operation expanded original missing set")
		}
		_, claim, _ := recoveryPaths(m, mh, next.Operation)
		if _, e := os.Stat(claim); !os.IsNotExist(e) {
			t.Fatal("rejected missing-set change consumed operation")
		}
	}
	a.Operation = "125-1"
	if e := consumeApproval(m, a, fingerprint([]byte("legitimate independent continuation")), initial, missing); e != nil {
		t.Fatal(e)
	}
	after, e := readRecoveryBoundary(m, a, mh)
	raw, _ := json.Marshal(after)
	if e != nil || string(raw) != string(before) {
		t.Fatal("legitimate continuation rewrote first boundary", e)
	}
}

func TestRecoveryApprovalRejectsUnboundResponsibilityAndTime(t *testing.T) {
	for name, change := range map[string]func(*approval){
		"missing freeze":    func(a *approval) { a.FreezeHash = "" },
		"not frozen":        func(a *approval) { a.RuntimeFrozen = false },
		"started":           func(a *approval) { a.RuntimeNotStarted = false },
		"wrong resource":    func(a *approval) { a.DResources = fingerprint([]byte("other")) },
		"wrong owner":       func(a *approval) { a.RecoveryOwner = "other" },
		"short retention":   func(a *approval) { a.RetentionDays = 29 },
		"expired":           func(a *approval) { a.Deadline = time.Now().Add(-time.Second).Format(time.RFC3339Nano) },
		"extended recovery": func(a *approval) { a.Deadline = time.Now().Add(2 * time.Hour).Format(time.RFC3339Nano) },
	} {
		t.Run(name, func(t *testing.T) {
			m, a, mh, _, _ := recoveryInputs(t)
			change(&a)
			if validateApproval(a, m, mh, testTool, time.Now()) == nil {
				t.Fatal("unproved approval accepted")
			}
		})
	}
}

func TestMaterialCopyDoesNotMoveDurableBoundaryNamespace(t *testing.T) {
	m, a, mh, p, ah := recoveryInputs(t)
	if consumeApproval(m, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) != nil {
		t.Fatal("first claim failed")
	}
	raw, e := readPrivate(m.CapturePath, mh)
	copyPath := filepath.Join(filepath.Dir(p), "copied-capture.json")
	if e != nil || writeExclusive(copyPath, raw) != nil {
		t.Fatal("owned material copy unavailable")
	}
	// Moving the external file cannot change the canonical path inside its
	// frozen bytes. The eventual CLI checks this before opening connections.
	var copied material
	if strictDecode(raw, &copied) != nil || copied.CapturePath == copyPath || copied.CapturePath != m.CapturePath {
		t.Fatal("material's canonical path changed")
	}
	if consumeApproval(copied, a, ah, ledgerRow{103, false}, []bool{true, true, true, true, true}) == nil {
		t.Fatal("copy bypassed consumed operation")
	}
}
