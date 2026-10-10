package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func TestFinalHistoricalEntryReadbackRetainsOriginalConclusionAndRejectsDrift(t *testing.T) {
	binding := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
	row := qualifiedCASEntryFixture("evaluation.requested")
	row.candidate.HistoricalGaps = []string{"storage_precision_gap"}
	stored, e := qualifiedCASEntry(binding, row, time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if finalMatchEventEntry(binding, row, stored) != nil || stored.Proof.Class != evidence.Unverifiable {
		t.Fatal("accepted pure historical gap changed")
	}
	for _, tc := range []struct {
		name   string
		change func(*evidence.HistoricalReferenceEntryV1)
	}{
		{"source_bytes", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.Digest.SHA256 = strings.Repeat("9", 64) }},
		{"binding", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.BusinessBindingSHA256 = strings.Repeat("9", 64) }},
		{"operation", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.Verification.OperationID = "124-1" }},
		{"verifier", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.Verification.Version = "different" }},
		{"conclusion", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.Class = evidence.RetiredVerified }},
		{"reason", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof.Verification.Reason = "unknown_execution" }},
		{"event_id", func(v *evidence.HistoricalReferenceEntryV1) { v.EventID = "different" }},
		{"missing_proof", func(v *evidence.HistoricalReferenceEntryV1) { v.Proof = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := stored.Clone()
			tc.change(&v)
			if finalMatchEventEntry(binding, row, v) == nil {
				t.Fatal("changed persisted original accepted")
			}
		})
	}
	row.candidate.BlockingReasons = []string{"unknown_execution"}
	if finalMatchEventEntry(binding, row, stored) == nil {
		t.Fatal("unknown promoted to historical gap")
	}
}

func TestFinalHistoricalAIRetainsPriorQFingerprintWithoutMutatingFreshQualification(t *testing.T) {
	_, _, expected := aiPersistenceTombstoneFixture(t)
	expected.References = append(expected.References, store.CommandRetirementReference{Kind: "migration_manifest", ID: strings.Repeat("a", 64)})
	actual := expected
	actual.References = append([]store.CommandRetirementReference(nil), expected.References...)
	actual.References[len(actual.References)-1].ID = strings.Repeat("b", 64)
	actual.VerifiedAt = actual.VerifiedAt.Add(-time.Hour)
	before, _ := json.Marshal(expected)
	if finalMatchRetirementEvidence(expected, actual) != nil {
		t.Fatal("original time/Q fingerprint relabelled")
	}
	after, _ := json.Marshal(expected)
	if !bytes.Equal(before, after) {
		t.Fatal("fresh opaque qualification was mutated")
	}
	for _, tc := range []struct {
		name   string
		change func(*store.CommandRetirementEvidence)
	}{
		{"owner", func(v *store.CommandRetirementEvidence) { v.OrganizationID = "8" }},
		{"admission", func(v *store.CommandRetirementEvidence) { v.AdmissionRevision++ }},
		{"source", func(v *store.CommandRetirementEvidence) { v.Sources[0].BytesSHA256 = strings.Repeat("9", 64) }},
		{"manifest_type", func(v *store.CommandRetirementEvidence) { v.References[len(v.References)-1].Kind = "business_record" }},
		{"manifest_id", func(v *store.CommandRetirementEvidence) { v.References[len(v.References)-1].ID = "invalid" }},
		{"unknown", func(v *store.CommandRetirementEvidence) { v.Reason = "unknown_execution" }},
		{"open", func(v *store.CommandRetirementEvidence) { v.ResponsibilityClosed = false }},
		{"verifier", func(v *store.CommandRetirementEvidence) { v.VerifierVersion = "different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := actual
			v.Sources = append([]store.CommandRetirementSource(nil), actual.Sources...)
			v.References = append([]store.CommandRetirementReference(nil), actual.References...)
			tc.change(&v)
			if finalMatchRetirementEvidence(expected, v) == nil {
				t.Fatal("changed stored retirement accepted")
			}
		})
	}
}

func TestFinalHistoricalNativeObservationCannotImportCompletion(t *testing.T) {
	for _, o := range []*FinalHistoricalObservation{nil, {}} {
		if o.ValidateBorrowedSnapshot(context.Background()) == nil || !reflect.DeepEqual(o.Summary(), FinalHistoricalSummary{}) {
			t.Fatal("zero observation adopted")
		}
		if _, e := json.Marshal(o); o != nil && e == nil {
			t.Fatal("observation serializable")
		}
	}
	for _, copies := range [][]SourceCopyInput{nil, make([]SourceCopyInput, 4), {{Input: bytes.NewBufferString("unseekable")}, {}, {}, {}}} {
		if o, e := PrepareFinalHistoricalEOF(t.Context(), FinalHistoricalInput{Copies: copies}); e == nil || o != nil {
			t.Fatal("copies/DTO minted native completion")
		}
	}
}
