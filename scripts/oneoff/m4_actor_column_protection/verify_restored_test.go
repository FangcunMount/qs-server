package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVerifyRestoredRequiresOriginalConsumedAuthority(t *testing.T) {
	m, _, hash, path, ah := recoveryInputs(t)
	args := actorConsumerArgs(m, hash, "restore", path, ah)
	args[0] = "--action=verify-restored"
	args = append(args, unitCaptureEvidence(t, m, hash, "readonly-verify")...)
	if _, e := parseArgs(args); e != nil {
		t.Fatal("read-only restored verifier rejects independently bound original recovery", e)
	}
}

func TestVerifyRestoredComparesEveryOriginalFactIncludingSQLNull(t *testing.T) {
	m := fixtureMaterial(t)
	current := cloneMaterial(t, m)
	current.CapturePath = ""
	current.CapturedAt = "2026-10-16T00:00:00Z"
	if e := verifyRestored(m, current); e != nil {
		t.Fatal("fresh exact observation rejected", e)
	}
	cases := map[string]func(*material){
		"clean102":          func(x *material) { x.Ledger[0].Version = 102 },
		"dirty101":          func(x *material) { x.Ledger[0].Dirty = true },
		"extra_ledger":      func(x *material) { x.Ledger = append(x.Ledger, ledgerRow{102, false}) },
		"ledger_definition": func(x *material) { x.LedgerDDL += " changed" },
		"mysql_identity":    func(x *material) { x.Binding.SQLIdentity = fingerprint([]byte("other")) },
		"mongo_identity":    func(x *material) { x.Binding.MongoIdentity = fingerprint([]byte("other")) },
		"mongo_uuid":        func(x *material) { x.Binding.MongoUUID = fingerprint([]byte("other")) },
		"mongo_raw":         func(x *material) { x.MongoHead = append(x.MongoHead, 0) },
		"sql_null":          func(x *material) { x.Tables[0].Rows[0].Value = &originalValue{Kind: "sql_null"} },
		"json_null":         func(x *material) { x.Tables[0].Rows[0].Value = &originalValue{Kind: "json", JSON: []byte("null")} },
		"raw_json":          func(x *material) { x.Tables[0].Rows[0].Value.JSON = []byte("[ ]") },
		"roles":             func(x *material) { x.Tables[1].Rows[0].Value.JSON = []byte(`["unexpected"]`) },
		"key":               func(x *material) { x.Tables[0].Rows[0].Key = "1" },
		"range":             func(x *material) { x.Tables[0].Rows = append(x.Tables[0].Rows, x.Tables[0].Rows[0]) },
		"eof":               func(x *material) { x.Tables[2].EOF = false },
		"pages":             func(x *material) { x.Tables[3].Pages++ },
		"archive":           func(x *material) { x.Tables[3].Rows[0].Retained = fingerprint([]byte("changed")) },
		"task":              func(x *material) { x.Tables[2].Rows[0].Retained = fingerprint([]byte("changed")) },
		"column":            func(x *material) { x.Tables[1].DDL = x.Tables[1].RetainedDDL },
		"retained_columns":  func(x *material) { x.Tables[0].Columns[0].Type = "bigint" },
		"session":           func(x *material) { x.SessionZone = "+08:00" },
		"tool":              func(x *material) { x.ToolSource = cSource },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			x := cloneMaterial(t, current)
			change(&x)
			if verifyRestored(m, x) == nil {
				t.Fatal("changed original state accepted")
			}
		})
	}
}

func TestVerifyRestoredRequiresConsumedOriginalOperationBeforeEnvironment(t *testing.T) {
	for _, kind := range []string{"no_boundary", "unconsumed_operation", "approval_reencoded", "lost_claim", "lost_journal", "lost_boundary", "marker", "expired", "material", "capture_proof", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			m, a, hash, ap, ah := recoveryInputs(t)
			evidenceArgs := unitCaptureEvidence(t, m, hash, "success")
			if kind != "no_boundary" {
				if e := consumeApproval(m, a, ah); e != nil {
					t.Fatal(e)
				}
			}
			boundary, claim, _ := recoveryPaths(m, hash, a.Operation)
			journal, _ := recoveryJournalPath(m, hash)
			switch kind {
			case "unconsumed_operation":
				a.Operation = "124-1"
			case "approval_reencoded":
				raw, e := json.MarshalIndent(a, "", "  ")
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(ap, raw, 0600); e != nil {
					t.Fatal(e)
				}
				ah = fingerprint(raw)
			case "lost_claim":
				if e := os.Remove(claim); e != nil {
					t.Fatal(e)
				}
			case "lost_journal":
				if e := os.Remove(journal); e != nil {
					t.Fatal(e)
				}
			case "lost_boundary":
				if e := os.Remove(boundary); e != nil {
					t.Fatal(e)
				}
			case "marker":
				if e := writeExclusive(a.Marker, []byte("runtime command issued")); e != nil {
					t.Fatal(e)
				}
			case "expired":
				a.Deadline = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
			case "material":
				if e := os.WriteFile(m.CapturePath, []byte("{}"), 0600); e != nil {
					t.Fatal(e)
				}
			case "capture_proof":
				if e := os.Remove(strings.TrimPrefix(evidenceArgs[0], "--capture-evidence-file=")); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "unconsumed_operation" || kind == "expired" {
				raw, _ := json.Marshal(a)
				if e := os.WriteFile(ap, raw, 0600); e != nil {
					t.Fatal(e)
				}
				ah = fingerprint(raw)
			}
			args := actorConsumerArgs(m, hash, "restore", ap, ah)
			args[0] = "--action=verify-restored"
			args = append(args, evidenceArgs...)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			r := run(ctx, args, func(string) string { t.Fatal("unproved readonly authority accessed connection environment"); return "" }, fixtureSource)
			if r.Complete || r.ApprovalConsumed || !r.ReadOnly || r.SQLConnectionClosed || r.SQLPoolClosed || r.MongoClosed {
				t.Fatal("unproved observation claimed execution or consumed authority", r)
			}
		})
	}
}

func TestVerifyRestoredReadsExistingResponsibilityWithoutChangingIt(t *testing.T) {
	m, a, hash, ap, ah := recoveryInputs(t)
	ev := unitCaptureEvidence(t, m, hash, "success")
	ep, eh := strings.TrimPrefix(ev[0], "--capture-evidence-file="), strings.TrimPrefix(ev[1], "--capture-evidence-sha256=")
	if e := consumeApproval(m, a, ah); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Dir(ap)
	before := privateDirectoryBytes(t, dir)
	for i := 0; i < 2; i++ {
		if e := verifyConsumedRecoveryInputs(t.Context(), m, a, m.CapturePath, hash, ap, ah, ep, eh, fixtureSource); e != nil {
			t.Fatal("consumed responsibility rejected", e)
		}
	}
	after := privateDirectoryBytes(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only observation created or changed original responsibility")
	}
	// A new operation is still unconsumed after repeated observations.
	next := a
	next.Operation = "124-1"
	_, claim, _ := recoveryPaths(m, hash, next.Operation)
	if _, e := os.Lstat(claim); !os.IsNotExist(e) {
		t.Fatal("observation created a claim")
	}
}

func privateDirectoryBytes(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		out[entry.Name()] = fingerprint(raw)
	}
	return out
}
