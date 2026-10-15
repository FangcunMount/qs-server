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

	"go.mongodb.org/mongo-driver/bson"
)

func currentRestoreApproval(t *testing.T, m *material, dir string) (approval, string, string, string, string) {
	t.Helper()
	now := time.Now().UTC()
	m.CapturedAt = now.Add(-time.Minute).Format(time.RFC3339Nano)
	materialPath := filepath.Join(dir, "original.json")
	raw, _ := json.Marshal(m)
	if e := writeExclusive(materialPath, raw); e != nil {
		t.Fatal(e)
	}
	materialHash := fingerprint(raw)
	a := approval{Format: 1, Kind: "m4_b_pre_start_mongo_object_restore", Operation: "123-1", ToolSource: testSource, BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, MaterialHash: materialHash, FreezeHash: strings.Repeat("d", 64), ResponsibilityHash: strings.Repeat("e", 64), SQLIdentity: m.SQLIdentity, MongoIdentity: m.MongoIdentity, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(dir, "first-runtime"), WindowStarted: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Add(-time.Second).Format(time.RFC3339Nano), Deadline: now.Add(30 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(time.Hour).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	approvalPath := filepath.Join(dir, "restore-approval.json")
	raw, _ = json.Marshal(a)
	if e := writeExclusive(approvalPath, raw); e != nil {
		t.Fatal(e)
	}
	return a, materialPath, materialHash, approvalPath, fingerprint(raw)
}
func TestVerifyTargetRequiresClean40AndExactRemainingScope(t *testing.T) {
	m := fixtureMaterial(t)
	o := currentState(t, m, 4, false)
	if e := verifyTargetObservation(m, o); e != nil {
		t.Fatal(e)
	}
	for missing := 0; missing < len(targets); missing++ {
		if verifyTargetObservation(m, currentState(t, m, missing, false)) == nil {
			t.Fatal("clean40 partial/full index restoration accepted as B installation", missing)
		}
	}
	for name, mutate := range map[string]func(*observation){
		"dirty40":           func(o *observation) { o.Ledger.Document = changeSpec(t, o.Ledger.Document, "dirty", true) },
		"one_index_present": func(o *observation) { o.Collections[0].Indexes = append(o.Collections[0].Indexes, targetSpec(t, 0)) },
		"count_changed":     func(o *observation) { n := int64(18); o.Collections[1].Count = &n },
		"old_ledger_uuid":   func(o *observation) { o.Ledger.Namespace.UUID = m.Ledger.Namespace.UUID },
	} {
		t.Run(name, func(t *testing.T) {
			bad := currentState(t, m, 4, false)
			mutate(&bad)
			if verifyTargetObservation(m, bad) == nil {
				t.Fatal("target proof accepted incomplete/mutated state")
			}
		})
	}
}
func TestLedgerReplayMustRetainProvider40UUIDAndCompleteOriginalBSON(t *testing.T) {
	m := fixtureMaterial(t)
	before := currentState(t, m, 2, true)
	after := currentState(t, m, 0, true)
	after.Ledger.Document = append([]byte(nil), m.Ledger.Document...)
	if e := verifyRestoredObservation(m, before, after); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*observation){
		"recreated_collection_uuid": func(o *observation) { o.Ledger.Namespace.UUID = m.Ledger.Namespace.UUID },
		"synthetic_39_document": func(o *observation) {
			o.Ledger.Document = rawBSON(t, bson.D{{Key: "_id", Value: int32(1)}, {Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}})
		},
		"same_number_different_type": func(o *observation) { o.Ledger.Document = changeSpec(t, m.Ledger.Document, "version", int32(39)) },
		"missing_target":             func(o *observation) { o.Collections[0].Indexes = o.Collections[0].Indexes[:2] },
	} {
		t.Run(name, func(t *testing.T) {
			bad := currentState(t, m, 0, true)
			bad.Ledger.Document = append([]byte(nil), m.Ledger.Document...)
			mutate(&bad)
			if verifyRestoredObservation(m, before, bad) == nil {
				t.Fatal("metadata replay rewrote original physical/BSON identity")
			}
		})
	}
}
func TestRecoveryDDLUsesOnlyFixedCapturedSpecAndRemainingAbsoluteBudget(t *testing.T) {
	m := fixtureMaterial(t)
	for i, target := range targets {
		cmd, e := originalIndexCommand(m, i, 37*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		raw := rawBSON(t, cmd)
		if bson.Raw(raw).Lookup("createIndexes").StringValue() != target.collection || bson.Raw(raw).Lookup("maxTimeMS").Int64() != 37000 {
			t.Fatal("DDL fixed scope/budget changed")
		}
		indexes := bson.Raw(raw).Lookup("indexes").Array()
		values, e := indexes.Values()
		if e != nil || len(values) != 1 || !bytes.Equal(values[0].Document(), targetSpec(t, i)) {
			t.Fatal("original ordered spec rewritten")
		}
	}
	for _, duration := range []time.Duration{0, 999 * time.Microsecond, 21 * time.Minute} {
		if _, e := originalIndexCommand(m, 0, duration); e == nil {
			t.Fatal("unbounded/exhausted DDL budget accepted")
		}
	}
	for _, position := range []int{-1, 4} {
		if _, e := originalIndexCommand(m, position, time.Second); e == nil {
			t.Fatal("uncompiled target accepted")
		}
	}
	ctx, c := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer c()
	left, e := recoveryBudget(ctx, time.Now())
	if e != nil || left <= 0 || left > 250*time.Millisecond {
		t.Fatal("absolute remaining budget lost")
	}
	if _, e := recoveryBudget(context.Background(), time.Now()); e == nil {
		t.Fatal("no deadline allowed")
	}
}
func TestRestorePreclaimRechecksExternalBytesMarkerAndSingleUse(t *testing.T) {
	m := fixtureMaterial(t)
	a, mp, mh, ap, ah := currentRestoreApproval(t, &m, privateDir(t))
	if e := rereadApproval(m, a, mp, mh, ap, ah, testSource); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(a.Marker, []byte("start intent"), 0600); e != nil {
		t.Fatal(e)
	}
	if rereadApproval(m, a, mp, mh, ap, ah, testSource) == nil {
		t.Fatal("late first-runtime marker ignored")
	}
	if _, e := os.Stat(ap + ".consumed"); !os.IsNotExist(e) {
		t.Fatal("preclaim failure consumed approval")
	}
	// Separate, untouched window: consumption is O_EXCL and bound to original
	// operation/material/raw approval. No retry can consume it a second time.
	m = fixtureMaterial(t)
	a, mp, mh, ap, ah = currentRestoreApproval(t, &m, privateDir(t))
	if e := rereadApproval(m, a, mp, mh, ap, ah, testSource); e != nil {
		t.Fatal(e)
	}
	if e := consumeApproval(ap, a, ah); e != nil {
		t.Fatal(e)
	}
	if consumeApproval(ap, a, ah) == nil {
		t.Fatal("approval reused")
	}
	if e := os.WriteFile(mp, []byte(`{"wrong":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	if rereadApproval(m, a, mp, mh, ap, ah, testSource) == nil {
		t.Fatal("changed original material accepted")
	}
}
