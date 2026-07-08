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
		case evaldomain.ModelKindCognitive:
			out[domain.KindCognitive] = true
		}
	}
	return out
}

func TestDefaultEvaluationDescriptorsAreExecutableRuntimeOnly(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	if len(descs) != 4 {
		t.Fatalf("descriptor count = %d, want 4 (scale + typology + behavioral_rating + cognitive)", len(descs))
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
	if kinds[evaldomain.ModelKindCognitive] != 1 {
		t.Fatalf("cognitive descriptor count = %d, want 1", kinds[evaldomain.ModelKindCognitive])
	}
	if descs[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first descriptor kind = %#v, want scale", descs[0].Kind)
	}
	if descs[1].Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("second descriptor algorithm = %#v, want configured typology", descs[1].Algorithm)
	}
	if descs[2].Kind != evaldomain.ModelKindBehavioralRating {
		t.Fatalf("third descriptor kind = %#v, want behavioral_rating", descs[2].Kind)
	}
	if descs[3].Kind != evaldomain.ModelKindCognitive {
		t.Fatalf("fourth descriptor kind = %#v, want cognitive", descs[3].Kind)
	}
}

func TestEvaluationDescriptorsMatchCapabilityPolicy(t *testing.T) {
	t.Parallel()

	descKinds := descriptorDomainKinds(DefaultEvaluationDescriptors())
	for _, kind := range domain.RuntimeExecutableKinds() {
		cap, ok := domain.FamilyCapabilityByKind(kind)
		if !ok {
			t.Fatalf("missing capability for %q", kind)
		}
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

	for _, kind := range domain.RuntimeExecutableKinds() {
		cap, ok := domain.FamilyCapabilityByKind(kind)
		if !ok {
			t.Fatalf("missing capability for %q", kind)
		}
		familyCap := cap
		t.Run(string(familyCap.Kind), func(t *testing.T) {
			t.Parallel()

			if got := descKinds[familyCap.Kind]; got != familyCap.RuntimeExecutable {
				t.Fatalf("descriptor presence = %v, want runtime executable %v (%s)", got, familyCap.RuntimeExecutable, familyCap.ExecutionPath)
			}

			_, _, _, legacyMapped := domain.LegacyKindMapping(familyCap.Kind)
			wantLegacy := familyCap.Kind == domain.KindScale
			if legacyMapped != wantLegacy {
				t.Fatalf("LegacyKindMapping(%q) = %v, want %v", familyCap.Kind, legacyMapped, wantLegacy)
			}

			_, evaluatorMapped := evaldomain.ExecutionIdentityFromLegacyKind(familyCap.Kind)
			if evaluatorMapped != wantLegacy {
				t.Fatalf("ExecutionIdentityFromLegacyKind(%q) = %v, want %v", familyCap.Kind, evaluatorMapped, wantLegacy)
			}
		})
	}
}

func TestBehavioralRatingRegistersDedicatedRuntimeDescriptor(t *testing.T) {
	t.Parallel()

	cap, ok := domain.FamilyCapabilityByKind(domain.KindBehavioralRating)
	if !ok || !cap.RuntimeExecutable {
		t.Fatalf("behavioral_rating capability = %#v", cap)
	}

	found := false
	for _, desc := range DefaultEvaluationDescriptors() {
		if desc.Kind == evaldomain.ModelKindBehavioralRating {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected behavioral_rating runtime descriptor")
	}
}
