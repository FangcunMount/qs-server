package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestAPIKindMapperAcceptsOnlyCanonicalKinds(t *testing.T) {
	t.Parallel()

	for apiKind, want := range map[string]domain.Kind{
		KindScale:            domain.KindScale,
		KindTypology:         domain.KindTypology,
		KindBehavioralRating: domain.KindBehavioralRating,
		KindCognitive:        domain.KindCognitive,
	} {
		got, ok := APIKindToDomainKind(apiKind)
		if !ok || got != want {
			t.Fatalf("APIKindToDomainKind(%q) = %q, %v; want %q, true", apiKind, got, ok, want)
		}
	}
	for _, legacy := range []string{"medical_scale", "personality", "behavior_ability", "custom"} {
		if _, ok := APIKindToDomainKind(legacy); ok {
			t.Fatalf("APIKindToDomainKind(%q) unexpectedly succeeded", legacy)
		}
		if IsSupportedAPIKind(legacy) {
			t.Fatalf("IsSupportedAPIKind(%q) = true", legacy)
		}
	}
}

func TestBehavioralRatingKindMapperBoundary(t *testing.T) {
	t.Parallel()

	domainKind, ok := APIKindToDomainKind(string(domain.KindBehavioralRating))
	if !ok || domainKind != domain.KindBehavioralRating {
		t.Fatalf("APIKindToDomainKind(behavioral_rating) = %q, %v", domainKind, ok)
	}
	if got := DomainKindToAPIKind(domain.KindBehavioralRating); got != string(domain.KindBehavioralRating) {
		t.Fatalf("DomainKindToAPIKind(behavioral_rating) = %q, want behavioral_rating", got)
	}
}
