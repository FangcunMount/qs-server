package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestBehaviorAbilityKindMapperBoundary(t *testing.T) {
	t.Parallel()

	domainKind, ok := APIKindToDomainKind(KindBehaviorAbility)
	if !ok || domainKind != domain.KindBehavioralRating {
		t.Fatalf("APIKindToDomainKind(behavior_ability) = %q, %v", domainKind, ok)
	}
	if got := DomainKindToAPIKind(domain.KindBehavioralRating); got != KindBehaviorAbility {
		t.Fatalf("DomainKindToAPIKind(behavioral_rating) = %q, want %q", got, KindBehaviorAbility)
	}
	if !domain.IsBehaviorAbilityScaleAdapter(domainKind) {
		t.Fatal("behavior_ability must resolve to scale adapter taxonomy slot")
	}
}

func TestBehaviorAbilityPayloadFormatBoundary(t *testing.T) {
	t.Parallel()

	got := APIPayloadFormatToDomain(PayloadFormatScaleV1)
	if got != domain.PayloadFormatBehaviorAbilityScaleV1 {
		t.Fatalf("APIPayloadFormatToDomain() = %q, want %q", got, domain.PayloadFormatBehaviorAbilityScaleV1)
	}
	if got == domain.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatal("behavior_ability payload must not normalize to behavioral_rating.default")
	}

	roundTrip := DomainPayloadFormatToAPI(KindBehaviorAbility, got)
	if roundTrip != PayloadFormatScaleV1 {
		t.Fatalf("DomainPayloadFormatToAPI() = %q, want %q", roundTrip, PayloadFormatScaleV1)
	}
}

func TestCapabilityPolicyUsesBehaviorAbilityAPIKind(t *testing.T) {
	t.Parallel()

	cap, ok := domain.CapabilityByAPIKind(KindBehaviorAbility)
	if !ok {
		t.Fatal("CapabilityByAPIKind(behavior_ability) = false, want true")
	}
	if cap.Kind != domain.KindBehavioralRating {
		t.Fatalf("capability kind = %q, want behavioral_rating", cap.Kind)
	}
	if cap.ExecutionPath != "behavior_ability_scale_adapter" {
		t.Fatalf("execution path = %q", cap.ExecutionPath)
	}
}
