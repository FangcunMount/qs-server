package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestTypologyAlgorithmsEquivalent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		left, right binding.Algorithm
		want        bool
	}{
		{binding.AlgorithmMBTI, binding.AlgorithmMBTI, true},
		{binding.AlgorithmMBTI, binding.AlgorithmPersonalityTypology, true},
		{binding.AlgorithmPersonalityTypology, binding.AlgorithmSBTI, true},
		{binding.AlgorithmBigFive, binding.AlgorithmPersonalityTypology, true},
		{binding.AlgorithmMBTI, binding.AlgorithmSBTI, false},
		{binding.AlgorithmMBTI, binding.AlgorithmBigFive, false},
		{binding.AlgorithmSPM, binding.AlgorithmPersonalityTypology, false},
	}
	for _, tc := range cases {
		got := identity.TypologyAlgorithmsEquivalent(tc.left, tc.right)
		if got != tc.want {
			t.Fatalf("%s ~ %s = %v, want %v", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestTypologyAlgorithmLookupAlternates(t *testing.T) {
	t.Parallel()
	alts := identity.TypologyAlgorithmLookupAlternates(binding.AlgorithmMBTI)
	if len(alts) != 1 || alts[0] != binding.AlgorithmPersonalityTypology {
		t.Fatalf("mbti alts = %#v", alts)
	}
	alts = identity.TypologyAlgorithmLookupAlternates(binding.AlgorithmPersonalityTypology)
	if len(alts) != 3 {
		t.Fatalf("canonical alts = %#v", alts)
	}
}

func TestBehavioralAlgorithmsEquivalent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		left, right binding.Algorithm
		want        bool
	}{
		{binding.AlgorithmBehavioralRatingDefault, binding.AlgorithmBrief2, true},
		{binding.AlgorithmBrief2, binding.AlgorithmBehavioralRatingDefault, true},
		{binding.AlgorithmBehavioralRatingDefault, binding.AlgorithmSPMSensory, true},
		{binding.AlgorithmBrief2, binding.AlgorithmSPMSensory, false},
		{binding.AlgorithmBrief2, binding.AlgorithmBrief2, true},
	}
	for _, tc := range cases {
		got := identity.BehavioralAlgorithmsEquivalent(tc.left, tc.right)
		if got != tc.want {
			t.Fatalf("%s ~ %s = %v, want %v", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestBehavioralAlgorithmBackfillTarget(t *testing.T) {
	t.Parallel()
	got, reason, ok := identity.BehavioralAlgorithmBackfillTarget(binding.AlgorithmBehavioralRatingDefault, true, false, "")
	if !ok || got != binding.AlgorithmBrief2 || reason != "" {
		t.Fatalf("brief2 spec: got=%s reason=%s ok=%v", got, reason, ok)
	}
	_, reason, ok = identity.BehavioralAlgorithmBackfillTarget(binding.AlgorithmBehavioralRatingDefault, false, true, "")
	if ok || reason != "ambiguous_brief2_or_spm_sensory" {
		t.Fatalf("ambiguous: reason=%s ok=%v", reason, ok)
	}
	got, reason, ok = identity.BehavioralAlgorithmBackfillTarget(binding.AlgorithmBehavioralRatingDefault, false, true, binding.AlgorithmSPMSensory)
	if !ok || got != binding.AlgorithmSPMSensory {
		t.Fatalf("explicit spm: got=%s reason=%s ok=%v", got, reason, ok)
	}
	_, reason, ok = identity.BehavioralAlgorithmBackfillTarget(binding.AlgorithmBehavioralRatingDefault, false, false, "")
	if ok || reason != "requires_brief2_execution_or_norm_refs" {
		t.Fatalf("ineligible: reason=%s ok=%v", reason, ok)
	}
}
