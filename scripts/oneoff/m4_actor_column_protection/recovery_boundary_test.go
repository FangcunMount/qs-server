package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func privateRecoveryDir(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil || os.Chmod(p, 0700) != nil {
		t.Fatal("owned private fixture directory unavailable")
	}
	return p
}

func recoveryInputs(t *testing.T) (material, approval, string, string, string) {
	t.Helper()
	m := fixtureMaterial(t)
	now := time.Now().UTC()
	m.CapturedAt = now.Add(-time.Minute).Format(time.RFC3339Nano)
	dir := privateRecoveryDir(t)
	m.CapturePath = filepath.Join(dir, "capture.json")
	raw, _ := json.Marshal(m)
	if writeExclusive(m.CapturePath, raw) != nil {
		t.Fatal("original capture unavailable")
	}
	hash := fingerprint(raw)
	a := approval{Format: 1, Kind: "m4_c_pre_start_actor_column_restore", Operation: "123-1", ToolSource: fixtureSource, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, MaterialHash: hash, FreezeHash: fingerprint([]byte("freeze")), ResponsibilityHash: fingerprint([]byte("owner")), Binding: m.Binding, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(dir, "first-runtime"), WindowStarted: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Add(-time.Second).Format(time.RFC3339Nano), Deadline: now.Add(30 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(time.Hour).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	raw, _ = json.Marshal(a)
	p := filepath.Join(dir, "approval.json")
	if writeExclusive(p, raw) != nil {
		t.Fatal("independent approval unavailable")
	}
	return m, a, hash, p, fingerprint(raw)
}

func TestRecoveryBoundaryPinsFirstScopeAndConsumesOperationIdentity(t *testing.T) {
	m, a, hash, p, ah := recoveryInputs(t)
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	boundary, claim, _ := recoveryPaths(m, hash, a.Operation)
	first, e := readOwnedPrivate(boundary, 32768)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := readPrivate(p, ah)
	if writeExclusive(filepath.Join(filepath.Dir(p), "copied-approval.json"), raw) != nil || consumeApproval(m, a, ah) == nil {
		t.Fatal("approval rename bypassed operation consumption")
	}
	raw, _ = json.MarshalIndent(a, "", "  ")
	if fingerprint(raw) == ah || consumeApproval(m, a, fingerprint(raw)) == nil {
		t.Fatal("approval reencoding bypassed operation consumption")
	}
	if _, e := readOwnedPrivate(claim, 32768); e != nil {
		t.Fatal("durable claim missing", e)
	}
	a.Operation = "124-1"
	if e := consumeApproval(m, a, fingerprint([]byte("new approval bytes"))); e != nil {
		t.Fatal("same-boundary independent operation rejected", e)
	}
	after, _ := readOwnedPrivate(boundary, 32768)
	if fingerprint(first) != fingerprint(after) {
		t.Fatal("continuation rewrote original boundary")
	}
}

func TestRecoveryContinuationCannotResetFirstBoundary(t *testing.T) {
	for _, field := range []string{"deadline", "decision", "expiry", "marker", "freeze", "binding", "window"} {
		t.Run(field, func(t *testing.T) {
			m, a, hash, _, ah := recoveryInputs(t)
			if consumeApproval(m, a, ah) != nil {
				t.Fatal("first claim unavailable")
			}
			next := a
			next.Operation = "124-1"
			switch field {
			case "deadline":
				v, _ := time.Parse(time.RFC3339Nano, a.Deadline)
				next.Deadline = v.Add(time.Second).Format(time.RFC3339Nano)
			case "decision":
				v, _ := time.Parse(time.RFC3339Nano, a.RecoveryDecision)
				next.RecoveryDecision = v.Add(time.Millisecond).Format(time.RFC3339Nano)
			case "expiry":
				v, _ := time.Parse(time.RFC3339Nano, a.Expires)
				next.Expires = v.Add(time.Second).Format(time.RFC3339Nano)
			case "marker":
				if writeExclusive(a.Marker, []byte("actual first-runtime intent")) != nil {
					t.Fatal("marker fault unavailable")
				}
				next.Marker = filepath.Join(filepath.Dir(a.Marker), "different-marker")
			case "freeze":
				next.FreezeHash = fingerprint([]byte("changed freeze"))
			case "binding":
				next.Binding.MongoUUID = fingerprint([]byte("changed ledger"))
			case "window":
				v, _ := time.Parse(time.RFC3339Nano, a.WindowStarted)
				next.WindowStarted = v.Add(-time.Second).Format(time.RFC3339Nano)
			}
			if consumeApproval(m, next, fingerprint([]byte("new bytes"))) == nil {
				t.Fatal("first recovery scope silently reset")
			}
			_, claim, _ := recoveryPaths(m, hash, next.Operation)
			if _, e := os.Stat(claim); !os.IsNotExist(e) {
				t.Fatal("changed scope consumed an operation")
			}
		})
	}
}

func TestLostBoundaryCannotBeRebuiltFromNewApproval(t *testing.T) {
	for _, damage := range []string{"missing", "truncated", "mode"} {
		t.Run(damage, func(t *testing.T) {
			m, a, hash, _, ah := recoveryInputs(t)
			if consumeApproval(m, a, ah) != nil {
				t.Fatal("first claim unavailable")
			}
			path, _, _ := recoveryPaths(m, hash, a.Operation)
			switch damage {
			case "missing":
				if os.Remove(path) != nil {
					t.Fatal("owned fault unavailable")
				}
			case "truncated":
				if os.WriteFile(path, []byte("{"), 0600) != nil {
					t.Fatal("owned fault unavailable")
				}
			case "mode":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("owned fault unavailable")
				}
			}
			a.Operation = "124-1"
			if consumeApproval(m, a, fingerprint([]byte("new bytes"))) == nil {
				t.Fatal("lost state recreated")
			}
		})
	}
}
