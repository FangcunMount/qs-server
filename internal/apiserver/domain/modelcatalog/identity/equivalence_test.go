package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestTypologyAlgorithmBackfillTarget(t *testing.T) {
	t.Parallel()
	got, ok := identity.TypologyAlgorithmBackfillTarget(binding.AlgorithmMBTI)
	if !ok || got != binding.AlgorithmPersonalityTypology {
		t.Fatalf("got=%s ok=%v", got, ok)
	}
	if _, ok := identity.TypologyAlgorithmBackfillTarget(binding.AlgorithmPersonalityTypology); ok {
		t.Fatal("canonical should not backfill")
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
