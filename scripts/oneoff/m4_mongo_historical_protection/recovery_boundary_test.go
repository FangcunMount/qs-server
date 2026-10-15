package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFirstRecoveryBoundaryAndOperationCannotBeMovedOrReencoded(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	boundary, consumed, e := recoveryPaths(m, mh, a.Operation)
	if e != nil {
		t.Fatal(e)
	}
	first, e := readOwnedPrivate(boundary, 32768)
	if e != nil {
		t.Fatal(e)
	}
	claim, e := readOwnedPrivate(consumed, 32768)
	if e != nil || len(claim) == 0 {
		t.Fatal("operation identity claim was not durably written")
	}
	copyPath := filepath.Join(filepath.Dir(ap), "same-approval-another-name.json")
	raw, _ := readPrivate(ap, ah, 32768)
	if e := writeExclusive(copyPath, raw); e != nil {
		t.Fatal(e)
	}
	if e := rereadApproval(m, a, mp, mh, copyPath, ah, testSource); e != nil {
		t.Fatal("identical raw approval input unexpectedly invalid", e)
	}
	if consumeApproval(m, a, ah) == nil {
		t.Fatal("copying the approval pathname bypassed operation consumption")
	}
	reencoded, _ := json.MarshalIndent(a, "", "  ")
	if fingerprint(reencoded) == ah || consumeApproval(m, a, fingerprint(reencoded)) == nil {
		t.Fatal("same operation was reused through a different raw encoding")
	}
	fresh := a
	fresh.Operation = "124-1"
	if e := consumeApproval(m, fresh, fingerprint([]byte("new independently approved raw bytes"))); e != nil {
		t.Fatal("only-operation continuation was rejected", e)
	}
	after, e := readOwnedPrivate(boundary, 32768)
	if e != nil || fingerprint(first) != fingerprint(after) {
		t.Fatal("a continuation overwrote the first recovery boundary")
	}
}

func TestContinuationCannotChangeOriginalBoundaryOrReplaceStartedMarker(t *testing.T) {
	for _, field := range []string{"deadline", "decision", "expiry", "marker", "freeze", "window"} {
		t.Run(field, func(t *testing.T) {
			m := fixtureMaterial(t)
			a, _, mh, _, ah := currentRestoreApproval(t, &m, privateDir(t))
			if e := consumeApproval(m, a, ah); e != nil {
				t.Fatal(e)
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
				if e := writeExclusive(a.Marker, []byte("actual first-runtime intent")); e != nil {
					t.Fatal(e)
				}
				next.Marker = filepath.Join(filepath.Dir(a.Marker), "different-absent-marker")
			case "freeze":
				next.FreezeHash = fingerprint([]byte("different freeze"))
			case "window":
				v, _ := time.Parse(time.RFC3339Nano, a.WindowStarted)
				next.WindowStarted = v.Add(-time.Second).Format(time.RFC3339Nano)
			}
			if e := consumeApproval(m, next, fingerprint([]byte("new raw approval"))); e == nil {
				t.Fatal("original boundary was silently reset")
			}
			_, path, _ := recoveryPaths(m, mh, next.Operation)
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("changed boundary consumed a new operation")
			}
		})
	}
}

func TestLostOrDamagedRecoveryBoundaryCannotBeRecreated(t *testing.T) {
	for _, fault := range []string{"missing_after_claim", "truncated", "unknown_kind", "unsafe_mode"} {
		t.Run(fault, func(t *testing.T) {
			m := fixtureMaterial(t)
			a, _, mh, _, ah := currentRestoreApproval(t, &m, privateDir(t))
			if e := consumeApproval(m, a, ah); e != nil {
				t.Fatal(e)
			}
			path, _, _ := recoveryPaths(m, mh, a.Operation)
			switch fault {
			case "missing_after_claim":
				if os.Remove(path) != nil {
					t.Fatal("owned boundary deletion fault unavailable")
				}
			case "truncated":
				if os.WriteFile(path, []byte("{"), 0600) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
			case "unknown_kind":
				if os.WriteFile(path, []byte(`{"format_version":1,"kind":"unknown"}`), 0600) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
			case "unsafe_mode":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("owned boundary fault unavailable")
				}
			}
			a.Operation = "124-1"
			if consumeApproval(m, a, fingerprint([]byte("new raw"))) == nil {
				t.Fatal("lost or damaged state allowed boundary reconstruction")
			}
			if fault == "missing_after_claim" {
				if _, e := os.Stat(path); !os.IsNotExist(e) {
					t.Fatal("missing boundary was recreated")
				}
			}
		})
	}
}

func TestCopiedOrOldMaterialPathRejectedBeforeEnvironment(t *testing.T) {
	m := fixtureMaterial(t)
	_, mp, mh, _, _ := currentRestoreApproval(t, &m, privateDir(t))
	raw, _ := readPrivate(mp, mh, maxMaterialBytes)
	copyPath := filepath.Join(filepath.Dir(mp), "copied-material.json")
	if writeExclusive(copyPath, raw) != nil {
		t.Fatal("owned material copy unavailable")
	}
	for _, path := range []string{copyPath, filepath.Join(filepath.Dir(mp), "old-material.json")} {
		hash := mh
		if path != copyPath {
			m.CapturePath = ""
			old, _ := json.Marshal(m)
			if writeExclusive(path, old) != nil {
				t.Fatal("old missing-anchor material fixture unavailable")
			}
			hash = fingerprint(old)
		}
		r := run(context.Background(), []string{"--action=verify-target", "--mysql-database=" + m.SQLName, "--mongo-database=" + m.MongoName, "--material-file=" + path, "--material-sha256=" + hash}, func(string) string { t.Fatal("environment read before material path rejection"); return "" }, testSource, nil)
		if r.Complete || r.Error != "material_rejected" {
			t.Fatalf("copied/missing anchor accepted: %+v", r)
		}
	}
}
