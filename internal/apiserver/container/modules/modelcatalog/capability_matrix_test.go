package modelcatalog

import (
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// runtimeCapability documents which domain kinds the wired evaluation runtime can execute.
// behavioral_rating is intentionally absent: behavior_ability binds and executes via the
// legacy scale path (MedicalScaleID), not a dedicated evaluator descriptor.
type runtimeCapability struct {
	domainKind        domain.Kind
	hasDescriptor     bool
	executionPath     string
	legacyKindMapping bool
}

func expectedRuntimeCapabilities() []runtimeCapability {
	return []runtimeCapability{
		{
			domainKind:        domain.KindScale,
			hasDescriptor:     true,
			executionPath:     "scale_descriptor",
			legacyKindMapping: true,
		},
		{
			domainKind:        domain.KindPersonality,
			hasDescriptor:     true,
			executionPath:     "typology_descriptor",
			legacyKindMapping: false,
		},
		{
			domainKind:        domain.KindBehavioralRating,
			hasDescriptor:     false,
			executionPath:     "scale_legacy_binding",
			legacyKindMapping: false,
		},
		{
			domainKind:        domain.KindCognitive,
			hasDescriptor:     false,
			executionPath:     "none",
			legacyKindMapping: false,
		},
		{
			domainKind:        domain.KindCustom,
			hasDescriptor:     false,
			executionPath:     "none",
			legacyKindMapping: false,
		},
	}
}

func descriptorDomainKinds(descs []evaldomain.ModelDescriptor) map[domain.Kind]bool {
	out := make(map[domain.Kind]bool)
	for _, desc := range descs {
		switch desc.Kind {
		case evaldomain.ModelKindScale:
			out[domain.KindScale] = true
		case evaldomain.ModelKindTypology:
			out[domain.KindPersonality] = true
		}
	}
	return out
}

func TestDefaultEvaluationDescriptorsAreExecutableRuntimeOnly(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	if len(descs) != 2 {
		t.Fatalf("descriptor count = %d, want 2 (scale + typology)", len(descs))
	}

	kinds := make(map[evaldomain.ModelKind]int)
	for _, desc := range descs {
		kinds[desc.Kind]++
	}
	if kinds[evaldomain.ModelKindScale] != 1 {
		t.Fatalf("scale descriptor count = %d, want 1", kinds[evaldomain.ModelKindScale])
	}
	if kinds[evaldomain.ModelKindTypology] != 1 {
		t.Fatalf("typology descriptor count = %d, want 1", kinds[evaldomain.ModelKindTypology])
	}
	if descs[0].Key != evaldomain.EvaluatorKeyScaleDefault {
		t.Fatalf("first descriptor key = %#v, want scale default", descs[0].Key)
	}
	if descs[1].Key != evaldomain.EvaluatorKeyPersonalityTypology {
		t.Fatalf("second descriptor key = %#v, want configured typology", descs[1].Key)
	}
}

func TestRuntimeCapabilityMatrix(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	descriptorKinds := descriptorDomainKinds(descs)

	for _, tc := range expectedRuntimeCapabilities() {
		tc := tc
		t.Run(string(tc.domainKind), func(t *testing.T) {
			t.Parallel()

			if got := descriptorKinds[tc.domainKind]; got != tc.hasDescriptor {
				t.Fatalf("descriptor presence = %v, want %v (execution path %q)", got, tc.hasDescriptor, tc.executionPath)
			}

			_, _, _, legacyMapped := domain.LegacyKindMapping(tc.domainKind)
			if legacyMapped != tc.legacyKindMapping {
				t.Fatalf("LegacyKindMapping(%q) = %v, want %v", tc.domainKind, legacyMapped, tc.legacyKindMapping)
			}

			_, evaluatorMapped := evaldomain.EvaluatorKeyFromLegacyKind(tc.domainKind)
			if evaluatorMapped && !tc.legacyKindMapping {
				t.Fatalf("EvaluatorKeyFromLegacyKind(%q) unexpectedly succeeded", tc.domainKind)
			}
		})
	}
}

func TestBehaviorAbilityDoesNotRegisterDedicatedRuntimeDescriptor(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	for _, desc := range descs {
		if desc.Key.Kind == domain.KindBehavioralRating {
			t.Fatalf("unexpected behavioral_rating descriptor: %#v", desc.Key)
		}
	}

	if _, ok := evaldomain.EvaluatorKeyFromLegacyKind(domain.KindBehavioralRating); ok {
		t.Fatal("behavioral_rating must not resolve to a legacy evaluator key")
	}
}
