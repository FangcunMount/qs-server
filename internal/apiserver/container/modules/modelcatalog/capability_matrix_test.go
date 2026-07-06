package modelcatalog

import (
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func descriptorDomainKinds(descs []evaldomain.ModelDescriptor) map[domain.Kind]bool {
	out := make(map[domain.Kind]bool)
	for _, desc := range descs {
		switch desc.Kind {
		case evaldomain.ModelKindScale:
			out[domain.KindScale] = true
		case evaldomain.ModelKindTypology:
			out[domain.KindPersonality] = true
		case evaldomain.ModelKindBehavioralRating:
			out[domain.KindBehavioralRating] = true
		}
	}
	return out
}

func TestDefaultEvaluationDescriptorsAreExecutableRuntimeOnly(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	if len(descs) != 3 {
		t.Fatalf("descriptor count = %d, want 3 (scale + typology + behavioral_rating)", len(descs))
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
	if kinds[evaldomain.ModelKindBehavioralRating] != 1 {
		t.Fatalf("behavioral_rating descriptor count = %d, want 1", kinds[evaldomain.ModelKindBehavioralRating])
	}
	if descs[0].Key != evaldomain.EvaluatorKeyScaleDefault {
		t.Fatalf("first descriptor key = %#v, want scale default", descs[0].Key)
	}
	if descs[1].Key != evaldomain.EvaluatorKeyPersonalityTypology {
		t.Fatalf("second descriptor key = %#v, want configured typology", descs[1].Key)
	}
	if descs[2].Key != evaldomain.EvaluatorKeyBehavioralRatingDefault {
		t.Fatalf("third descriptor key = %#v, want behavioral_rating default", descs[2].Key)
	}
}

func TestEvaluationDescriptorsMatchCapabilityPolicy(t *testing.T) {
	t.Parallel()

	descKinds := descriptorDomainKinds(DefaultEvaluationDescriptors())
	for _, cap := range domain.DefaultCapabilities() {
		if cap.RuntimeExecutable && !descKinds[cap.Kind] {
			t.Fatalf("missing runtime descriptor for %q (%s)", cap.Kind, cap.ExecutionPath)
		}
		if !cap.RuntimeExecutable && descKinds[cap.Kind] {
			t.Fatalf("unexpected runtime descriptor for %q (%s)", cap.Kind, cap.ExecutionPath)
		}
	}
}

func TestRuntimeCapabilityPolicy(t *testing.T) {
	t.Parallel()

	descKinds := descriptorDomainKinds(DefaultEvaluationDescriptors())

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		t.Run(string(cap.Kind), func(t *testing.T) {
			t.Parallel()

			if got := descKinds[cap.Kind]; got != cap.RuntimeExecutable {
				t.Fatalf("descriptor presence = %v, want runtime executable %v (%s)", got, cap.RuntimeExecutable, cap.ExecutionPath)
			}

			_, _, _, legacyMapped := domain.LegacyKindMapping(cap.Kind)
			wantLegacy := cap.Kind == domain.KindScale
			if legacyMapped != wantLegacy {
				t.Fatalf("LegacyKindMapping(%q) = %v, want %v", cap.Kind, legacyMapped, wantLegacy)
			}

			_, evaluatorMapped := evaldomain.EvaluatorKeyFromLegacyKind(cap.Kind)
			if evaluatorMapped != wantLegacy {
				t.Fatalf("EvaluatorKeyFromLegacyKind(%q) = %v, want %v", cap.Kind, evaluatorMapped, wantLegacy)
			}
		})
	}
}

func TestBehaviorAbilityDoesNotRegisterDedicatedRuntimeDescriptor(t *testing.T) {
	t.Parallel()

	cap, ok := domain.CapabilityByKind(domain.KindBehaviorAbility)
	if !ok || cap.RuntimeExecutable || !cap.RuntimeViaScaleLegacy {
		t.Fatalf("behavior_ability capability = %#v", cap)
	}

	for _, desc := range DefaultEvaluationDescriptors() {
		if desc.Key.Kind == domain.KindBehaviorAbility {
			t.Fatalf("unexpected behavior_ability descriptor: %#v", desc.Key)
		}
	}
}

func TestBehavioralRatingRegistersDedicatedRuntimeDescriptor(t *testing.T) {
	t.Parallel()

	cap, ok := domain.CapabilityByKind(domain.KindBehavioralRating)
	if !ok || !cap.RuntimeExecutable || cap.RuntimeViaScaleLegacy {
		t.Fatalf("behavioral_rating capability = %#v", cap)
	}

	found := false
	for _, desc := range DefaultEvaluationDescriptors() {
		if desc.Key.Kind == domain.KindBehavioralRating {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected behavioral_rating runtime descriptor")
	}
}
