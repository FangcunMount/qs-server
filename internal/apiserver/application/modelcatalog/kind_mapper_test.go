package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestBehaviorAbilityIsProductChannelOnly(t *testing.T) {
	t.Parallel()

	if _, ok := APIKindToDomainKind(KindBehaviorAbility); ok {
		t.Fatal("APIKindToDomainKind(behavior_ability) should not map to a domain kind")
	}
	if !IsSupportedAPIKind(KindBehaviorAbility) {
		t.Fatal("behavior_ability must remain a supported API channel kind")
	}
	if !domain.IsBehaviorAbilityProductChannelAPIKind(KindBehaviorAbility) {
		t.Fatal("behavior_ability must be a product channel API kind")
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

func TestMedicalScalePayloadFormatBoundary(t *testing.T) {
	t.Parallel()

	got := APIPayloadFormatToDomain(PayloadFormatMedicalScaleV1)
	if got != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("APIPayloadFormatToDomain() = %q, want %q", got, domain.PayloadFormatAssessmentScaleV1)
	}
	roundTrip := DomainPayloadFormatToAPI(KindMedicalScale, got)
	if roundTrip != PayloadFormatMedicalScaleV1 {
		t.Fatalf("DomainPayloadFormatToAPI() = %q, want %q", roundTrip, PayloadFormatMedicalScaleV1)
	}
}
