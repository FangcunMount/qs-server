package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func historyEntry(id string) HistoricalReferenceEntryV1 {
	d := SourceDigest("mysql-source-row-v1", []byte(id))
	return HistoricalReferenceEntryV1{EventID: id, EventType: "evaluation.requested", Source: HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "mysql_uint64", PrimaryKeySHA256: SourceDigest("pk", []byte(id)).SHA256, Digest: d}, Proof: &EventEvidenceV1{Version: 1, Class: RetiredVerified, EventID: id, Digest: d, BusinessBindingSHA256: strings.Repeat("1", 64), Origin: "retirement", Verification: Verification{Method: "source-and-business", Version: "v1", OperationID: "123-1", VerifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
}

func TestHistoricalReferenceAppendAndIsolation(t *testing.T) {
	entry := historyEntry("event-one")
	entry.Run = &HistoricalRunReferenceV1{RunID: "run-one", Attempt: 1}
	var empty *HistoricalReferenceSetV1
	set, err := empty.Append(entry)
	if err != nil {
		t.Fatal(err)
	}
	again, err := set.Append(entry)
	if err != nil || !reflect.DeepEqual(set, again) {
		t.Fatalf("same proof not idempotent: %v", err)
	}
	entry.Proof.Verification.Reason = "caller mutation"
	entry.Run.RunID = "caller mutation"
	if set.Entries[0].Proof.Verification.Reason != "" || set.Entries[0].Run.RunID != "run-one" {
		t.Fatal("caller aliases persisted proof")
	}
	clone := set.Clone()
	clone.Entries[0].Proof.Verification.Reason = "clone mutation"
	if set.Entries[0].Proof.Verification.Reason != "" {
		t.Fatal("clone aliases persisted proof")
	}
	for _, change := range []func(*HistoricalReferenceEntryV1){func(e *HistoricalReferenceEntryV1) { e.EventType = "evaluation.failed" }, func(e *HistoricalReferenceEntryV1) { e.Source.PrimaryKeySHA256 = strings.Repeat("2", 64) }, func(e *HistoricalReferenceEntryV1) { e.Proof.Verification.Version = "v2" }, func(e *HistoricalReferenceEntryV1) { e.Run.Attempt = 2 }} {
		changed := set.Entries[0].Clone()
		change(&changed)
		if _, err := set.Append(changed); !errors.Is(err, ErrHistoricalReferenceConflict) {
			t.Fatalf("changed identity/proof accepted: %v", err)
		}
	}
}

func TestHistoricalReferenceValidation(t *testing.T) {
	for name, change := range map[string]func(*HistoricalReferenceEntryV1){
		"type":     func(e *HistoricalReferenceEntryV1) { e.EventType = "untrusted.type" },
		"engine":   func(e *HistoricalReferenceEntryV1) { e.Source.Database = "mongodb" },
		"object":   func(e *HistoricalReferenceEntryV1) { e.Source.Object = "rm_outbox" },
		"pk-kind":  func(e *HistoricalReferenceEntryV1) { e.Source.PrimaryKeyKind = "mongodb_objectid" },
		"pk":       func(e *HistoricalReferenceEntryV1) { e.Source.PrimaryKeySHA256 = "" },
		"digest":   func(e *HistoricalReferenceEntryV1) { e.Source.Digest.SHA256 = strings.Repeat("2", 64) },
		"nil":      func(e *HistoricalReferenceEntryV1) { e.Proof = nil },
		"standard": func(e *HistoricalReferenceEntryV1) { e.Proof.Class = StandardReferenceClass },
		"id":       func(e *HistoricalReferenceEntryV1) { e.Proof.EventID = "other" },
		"source-kind": func(e *HistoricalReferenceEntryV1) {
			e.Source.Digest.Kind = SDKFingerprintKind
			e.Proof.Digest.Kind = SDKFingerprintKind
		},
		"closure":   func(e *HistoricalReferenceEntryV1) { e.Proof.Verification.ResponsibilityClosed = false },
		"owner":     func(e *HistoricalReferenceEntryV1) { e.Proof.Verification.OwnershipVerified = false },
		"terminal":  func(e *HistoricalReferenceEntryV1) { e.Proof.Verification.BusinessTerminal = false },
		"operation": func(e *HistoricalReferenceEntryV1) { e.Proof.Verification.OperationID = "forged" },
		"precision": func(e *HistoricalReferenceEntryV1) {
			e.Proof.Verification.VerifiedAt = e.Proof.Verification.VerifiedAt.Add(time.Nanosecond)
		},
		"timezone": func(e *HistoricalReferenceEntryV1) {
			e.Proof.Verification.VerifiedAt = e.Proof.Verification.VerifiedAt.In(time.FixedZone("offset", 3600))
		},
		"attempt": func(e *HistoricalReferenceEntryV1) { e.Run = &HistoricalRunReferenceV1{RunID: "run", Attempt: 0} },
		"gap":     func(e *HistoricalReferenceEntryV1) { e.Proof.Class = Unverifiable },
	} {
		t.Run(name, func(t *testing.T) {
			e := historyEntry("event")
			change(&e)
			if e.Validate() == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
	gap := historyEntry("event")
	gap.Proof.Class = Unverifiable
	gap.Proof.Verification.Reason = "original_business_clock_absent"
	if err := gap.Validate(); err != nil {
		t.Fatal(err)
	}
	duplicate := &HistoricalReferenceSetV1{Version: 1, Entries: []HistoricalReferenceEntryV1{gap, gap}}
	if !errors.Is(duplicate.Validate(), ErrHistoricalReferenceConflict) {
		t.Fatal("duplicate raw references accepted")
	}
}

func TestHistoricalReferenceCapacityAndStrictJSON(t *testing.T) {
	entries := make([]HistoricalReferenceEntryV1, HistoricalReferenceMaxEntries)
	for i := range entries {
		entries[i] = historyEntry(fmt.Sprintf("event-%d", i))
	}
	set := &HistoricalReferenceSetV1{Version: 1, Entries: entries}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Append(historyEntry("overflow")); !errors.Is(err, ErrHistoricalReferenceLimit) {
		t.Fatalf("count overflow accepted: %v", err)
	}
	if _, err := set.Append(make([]HistoricalReferenceEntryV1, HistoricalReferenceMaxEntries+1)...); !errors.Is(err, ErrHistoricalReferenceLimit) {
		t.Fatal("oversized append input not rejected before iteration")
	}
	for i := range entries {
		entries[i].Proof.Verification.Reason = strings.Repeat("r", 4096)
	}
	if !errors.Is(set.Validate(), ErrHistoricalReferenceLimit) {
		t.Fatal("byte overflow accepted")
	}
	valid, err := (*HistoricalReferenceSetV1)(nil).Append(historyEntry("strict"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeHistoricalReferenceSetJSON(raw)
	if err != nil || !reflect.DeepEqual(valid, decoded) {
		t.Fatalf("JSON roundtrip failed: %v", err)
	}
	for _, bad := range []string{string(raw) + `{}`, strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(raw), `"version":1`, `"version":1,"unknown":true`, 1), `null`, `{"version":1,"entries":[]}`} {
		if _, err := DecodeHistoricalReferenceSetJSON([]byte(bad)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
