package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestM5RestoreApprovalIsSeparateBoundedAndPreStart(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hash := strings.Repeat("a", 64)
	cfg := M5RestoreConfig{Pair: PairConfig{ExpectedSourceSHA: strings.Repeat("b", 40)}, OriginalSHA256: hash}
	m := M5OriginalMaterial{OperationID: "owned-m5-restore"}
	a := M5RestoreApproval{FormatVersion: 1, Kind: "qs_m5_original_empty_restore", OperationID: m.OperationID, ApprovedSourceSHA: cfg.Pair.ExpectedSourceSHA, OriginalSHA256: hash, ObservedStateSHA256: hash, FreezeReceiptSHA256: hash, BoundaryReceiptSHA256: hash, ReviewSHA256: hash, ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), RecoveryStartedAt: now.Add(-time.Minute).Format(time.RFC3339), DataOwner: "shujie", RecoveryOwner: "shujie", RuntimeFrozen: true, RuntimeNotStarted: true}
	if err := validateM5RestoreApproval(a, m, cfg, hash, now); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*M5RestoreApproval){
		func(x *M5RestoreApproval) { x.Kind = "offline_upgrade" },
		func(x *M5RestoreApproval) { x.RuntimeNotStarted = false },
		func(x *M5RestoreApproval) { x.RuntimeFrozen = false },
		func(x *M5RestoreApproval) { x.OriginalSHA256 = strings.Repeat("c", 64) },
		func(x *M5RestoreApproval) { x.ObservedStateSHA256 = strings.Repeat("c", 64) },
		func(x *M5RestoreApproval) { x.ApprovedSourceSHA = strings.Repeat("c", 40) },
		func(x *M5RestoreApproval) { x.FreezeReceiptSHA256 = strings.Repeat("0", 64) },
		func(x *M5RestoreApproval) { x.BoundaryReceiptSHA256 = "" },
		func(x *M5RestoreApproval) { x.DataOwner = "" },
		func(x *M5RestoreApproval) { x.ExpiresAt = now.Format(time.RFC3339) },
		func(x *M5RestoreApproval) { x.RecoveryStartedAt = now.Add(-2 * time.Hour).Format(time.RFC3339) },
		func(x *M5RestoreApproval) { x.RecoveryStartedAt = now.Add(time.Second).Format(time.RFC3339) },
		func(x *M5RestoreApproval) { x.ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339) },
	} {
		bad := a
		change(&bad)
		if validateM5RestoreApproval(bad, m, cfg, hash, now) == nil {
			t.Fatal("unsafe approval accepted", bad)
		}
	}
}

func TestM5PrivateMaterialBoundAndExactJSON(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "material.json")
	for _, raw := range []string{`{"format_version":1,"format_version":1}`, `{"unexpected":true}`, `{} {}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		var out M5OriginalMaterial
		if decodeM5Private(path, retirementHash([]byte(raw)), 1<<20, &out) == nil {
			t.Fatal("ambiguous private JSON accepted")
		}
	}
	raw, _ := json.Marshal(map[string]string{"payload": strings.Repeat("a", 40000)})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateRetirementAuthorization(path, retirementHash(raw)); err == nil {
		t.Fatal("existing authorization limit weakened")
	}
	if _, err := readPrivateRetirementFile(path, retirementHash(raw), 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateRetirementFile(path, retirementHash(raw), 1<<20); err == nil {
		t.Fatal("public material accepted")
	}
}

func TestM5RetainedComparisonPreservesAllOtherObjects(t *testing.T) {
	n := uint64(1)
	a := map[string]RetainedObject{"qs.mysql.schema_migrations": {}, "qs.mysql.ai_messaging_outbox": {}, "qs.mysql.rm_tx_witness": {Kind: "BASE TABLE", Rows: &n, ContentSHA256: "original"}, "qs.mongo.business_ack": {Kind: "collection", Rows: &n, ContentSHA256: "original"}}
	b := map[string]RetainedObject{}
	for k, v := range a {
		b[k] = v
	}
	delete(b, "qs.mysql.ai_messaging_outbox")
	b["qs.mysql.schema_migrations"] = RetainedObject{ContentSHA256: "interrupted"}
	if err := compareM5Retained(a, b); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"qs.mysql.rm_tx_witness", "qs.mongo.business_ack"} {
		old := b[name]
		b[name] = RetainedObject{ContentSHA256: "changed"}
		if compareM5Retained(a, b) == nil {
			t.Fatal("retained change accepted")
		}
		b[name] = old
		delete(b, name)
		if compareM5Retained(a, b) == nil {
			t.Fatal("retained removal accepted")
		}
		b[name] = old
	}
	b["qs.mysql.new_fact"] = RetainedObject{}
	if compareM5Retained(a, b) == nil {
		t.Fatal("unreviewed new object accepted")
	}
}
