package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/payloadformat"
)

func TestResolveRuntimeIdentityFreezesCompatibleRoute(t *testing.T) {
	t.Parallel()

	got, err := identity.ResolveRuntimeIdentity(
		identity.KindScale, "", identity.AlgorithmScaleDefault,
		identity.DecisionKindScoreRange, payloadformat.PayloadFormatAssessmentScaleV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.AlgorithmFamily != identity.AlgorithmFamilyFactorScoring || !got.Complete() {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveRuntimeIdentityRejectsFamilyConflict(t *testing.T) {
	t.Parallel()

	_, err := identity.ResolveRuntimeIdentity(
		identity.KindBehavioralRating, "", identity.AlgorithmBrief2,
		identity.DecisionKindScoreRange, payloadformat.PayloadFormatBehavioralRatingDefaultV1,
	)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestResolveRuntimeIdentityRejectsLegacyFormat(t *testing.T) {
	t.Parallel()

	_, err := identity.ResolveRuntimeIdentity(
		identity.KindScale, "", identity.AlgorithmScaleDefault,
		identity.DecisionKindScoreRange, payloadformat.PayloadFormatScaleV1,
	)
	if err == nil {
		t.Fatal("expected legacy format error")
	}
}

func TestResolveRuntimeIdentityLegacyListMatchesPayloadFormat(t *testing.T) {
	t.Parallel()
	for _, format := range payloadformat.LegacyDecodeOnlyPayloadFormats() {
		_, err := identity.ResolveRuntimeIdentity(
			identity.KindScale, "", identity.AlgorithmScaleDefault,
			identity.DecisionKindScoreRange, format,
		)
		if err == nil {
			t.Fatalf("expected legacy reject for %q", format)
		}
	}
}
