package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func targetBReconcileFixture() (TargetBRecoveryResumeRequest, *targetJournalSnapshot) {
	s := targetBJournalFixture()
	s.hash = strings.Repeat("8", 64)
	r := TargetBRecoveryResumeRequest{Recovery: TargetRecoveryResumeRequest{Original: s.request, CurrentRunID: "126-1", JournalSHA256: s.hash, WindowStartSHA256: s.windowStart}, ApprovedBSourceSHA: strings.Repeat("f", 40), MigrationIntentSHA256: sha(s.files["target-recovery-b-migration-intent.json"]), MigrationResultSHA256: sha(s.files["target-recovery-b-migration-result.json"])}
	return r, s
}

func TestBRecoveryExpectedMaterialCannotAutoApproveObservation(t *testing.T) {
	r, s := targetBReconcileFixture()
	if _, _, e := targetBReconcileInput(r, s); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name   string
		change func(*TargetBRecoveryResumeRequest, *targetJournalSnapshot)
		want   error
	}{
		{"expected_b_source", func(r *TargetBRecoveryResumeRequest, _ *targetJournalSnapshot) {
			r.ApprovedBSourceSHA = strings.Repeat("9", 40)
		}, ErrRecoveryBinding},
		{"expected_intent_hash", func(r *TargetBRecoveryResumeRequest, _ *targetJournalSnapshot) {
			r.MigrationIntentSHA256 = strings.Repeat("9", 64)
		}, ErrRecoveryJournal},
		{"expected_result_hash", func(r *TargetBRecoveryResumeRequest, _ *targetJournalSnapshot) {
			r.MigrationResultSHA256 = strings.Repeat("9", 64)
		}, ErrRecoveryJournal},
		{"whole_journal", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) { s.hash = strings.Repeat("9", 64) }, ErrRecoveryBinding},
		{"original_op", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) { s.request.OperationID = "127-1" }, ErrRecoveryBinding},
		{"window_restart", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) {
			s.windowStart = strings.Repeat("9", 64)
		}, ErrRecoveryBinding},
		{"head_plus_one", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) { s.request.MongoHead = 39 }, ErrRecoveryBinding},
		{"missing_response", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) {
			delete(s.files, "target-recovery-b-migration-result.json")
		}, ErrRecoveryUnknown},
		{"partial_response", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) {
			var x targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &x)
			x.State = "partial_or_unknown"
			x.Observation = nil
			x.Attempt.Completion = "partial_or_unknown"
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(x)
		}, ErrRecoveryUnknown},
		// Consistently edited intent/result bytes cannot overwrite the fixed
		// expected physical hashes even if their internal grammar agrees.
		{"self_consistent_edit", func(_ *TargetBRecoveryResumeRequest, s *targetJournalSnapshot) {
			var x targetBMigrationIntent
			_ = json.Unmarshal(s.files["target-recovery-b-migration-intent.json"], &x)
			x.MongoStableSchemaSHA256 = strings.Repeat("9", 64)
			raw, _ := json.Marshal(x)
			var y targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &y)
			y.IntentSHA256 = sha(raw)
			s.files["target-recovery-b-migration-intent.json"] = raw
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(y)
		}, ErrRecoveryJournal},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, s := targetBReconcileFixture()
			test.change(&r, s)
			if _, _, e := targetBReconcileInput(r, s); e != test.want {
				t.Fatalf("category=%v want=%v", e, test.want)
			}
		})
	}
}

func TestBRecoveryRewindsOnlyObservedMigrationGenerationForComparison(t *testing.T) {
	oldUUID := make([]byte, 16)
	oldUUID[0] = 1
	before := targetBTestSchema(t, 1)
	after := targetBTestSchema(t, 2)
	original, _ := json.Marshal(after)
	want := targetMongoNonTarget(before)
	got, e := targetBOriginalMongoSchema(after, hex.EncodeToString(oldUUID))
	if e != nil || got != want {
		t.Fatalf("exact driver generation comparison failed: %v", e)
	}
	unchanged, _ := json.Marshal(after)
	if !reflect.DeepEqual(original, unchanged) {
		t.Fatal("native current metadata was mutated")
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"kept_uuid", func(x map[string]any) {
			x["collection:answersheets"].(map[string]any)["info"] = map[string]any{"uuid": "different"}
		}},
		{"kept_namespace_missing", func(x map[string]any) { delete(x, "collection:answersheets") }},
		{"migration_options", func(x map[string]any) {
			x["collection:schema_migrations"].(map[string]any)["options"] = map[string]any{"validator": map[string]any{"unexpected": true}}
		}},
		{"migration_index", func(x map[string]any) { x["indexes:schema_migrations"] = []any{map[string]any{"name": "extra"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			x := targetBTestSchema(t, 2)
			test.change(x)
			h, e := targetBOriginalMongoSchema(x, hex.EncodeToString(oldUUID))
			if e == nil && h == want {
				t.Fatal("non-target mutation disappeared")
			}
		})
	}
	for _, id := range []string{"", strings.Repeat("1", 31), strings.Repeat("g", 32)} {
		if _, e := targetBOriginalMongoSchema(after, id); e != ErrStructure {
			t.Fatal("unknown original UUID accepted")
		}
	}
}

func TestBRecoveryPhysicalOriginalEntriesCannotBeReplaced(t *testing.T) {
	dir := targetJournalDirectoryBinding{Device: 1, Inode: 2, UID: 501, GID: 20, Mode: 0700, Links: 2}
	entry := targetJournalEntryBinding{Name: "target-recovery-b-migration-result.json", SHA256: strings.Repeat("a", 64), Bytes: 32, Device: 1, Inode: 3, UID: 501, GID: 20, Mode: 0600, Links: 1, Modified: 4, Changed: 5}
	actual := &targetJournalSnapshot{directory: dir, entries: []targetJournalEntryBinding{entry}}
	if !targetBOriginalEntriesEqual([]targetJournalEntryBinding{entry}, actual, dir) {
		t.Fatal("original entries refused")
	}
	newEntry := entry
	newEntry.Name = "target-recovery-0-resume-binding.json"
	newEntry.Inode = 6
	actual.entries = append(actual.entries, newEntry)
	if !targetBOriginalEntriesEqual([]targetJournalEntryBinding{entry}, actual, dir) {
		t.Fatal("canonical append should not replace original")
	}
	for _, test := range []struct {
		name   string
		change func(*targetJournalSnapshot)
	}{
		{"directory_reused", func(s *targetJournalSnapshot) { s.directory.Inode++ }},
		{"inode_reused", func(s *targetJournalSnapshot) { s.entries[0].Inode++ }},
		{"content_changed", func(s *targetJournalSnapshot) { s.entries[0].SHA256 = strings.Repeat("b", 64) }},
		{"mode_changed", func(s *targetJournalSnapshot) { s.entries[0].Mode = 0644 }},
		{"link_changed", func(s *targetJournalSnapshot) { s.entries[0].Links = 2 }},
		{"entry_deleted", func(s *targetJournalSnapshot) { s.entries = s.entries[1:] }},
		{"duplicate_name", func(s *targetJournalSnapshot) { s.entries = append(s.entries, s.entries[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &targetJournalSnapshot{directory: dir, entries: []targetJournalEntryBinding{entry, newEntry}}
			test.change(s)
			if targetBOriginalEntriesEqual([]targetJournalEntryBinding{entry}, s, dir) {
				t.Fatal("old physical identity reused")
			}
		})
	}
}

func TestBRecoveryZeroProofAndSummaryCannotSupplyNativeHandles(t *testing.T) {
	for _, p := range []*targetBReconciledMigrationProof{nil, {}, {request: TargetBRecoveryResumeRequest{ApprovedBSourceSHA: strings.Repeat("a", 40)}}} {
		if e := p.VerifyAfter(context.Background(), nil, nil); e != ErrRecoveryBinding {
			t.Fatal("native identity reconstructed from material")
		}
		if _, e := p.MarshalJSON(); e != ErrSerialization {
			t.Fatal("process proof serialized")
		}
	}
	if _, e := ReconcileTargetBRecovery(context.Background(), nil, TargetRecoveryBorrowed{}, TargetBRecoveryResumeRequest{}, "/not-used", nil); e != ErrRecoveryBinding {
		t.Fatal("missing real inputs accepted")
	}
	var v TargetRecoveryReconciliation
	if _, e := v.reconcileActual(context.Background(), TargetRecoveryBorrowed{}, nil); e != ErrRecoveryBinding {
		t.Fatal("summary imported as reconciliation")
	}
}
