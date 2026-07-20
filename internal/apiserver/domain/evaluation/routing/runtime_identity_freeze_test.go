package evaluation_test

import (
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFrozenRuntimeIdentityDoesNotSilentFallback(t *testing.T) {
	t.Parallel()

	route := evalpipeline.ModelRoute{
		Kind:            modelcatalog.KindBehavioralRating,
		Algorithm:       modelcatalog.AlgorithmBrief2,
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		DecisionKind:    modelcatalog.DecisionKindScoreRange, // incompatible with factor_norm
		PayloadFormat:   modelcatalog.PayloadFormatBehavioralRatingDefaultV1,
	}
	if !route.HasFrozenRuntime() {
		t.Fatal("expected frozen runtime")
	}
	_, err := evalpipeline.DescriptorKeyFromRoute(route)
	if err == nil {
		t.Fatal("expected error when frozen decision conflicts with family")
	}
}

func TestFrozenRuntimeIdentityPreferredOverIdentityDerivation(t *testing.T) {
	t.Parallel()

	route := evalpipeline.ModelRoute{
		Kind:            modelcatalog.KindScale,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		PayloadFormat:   modelcatalog.PayloadFormatAssessmentScaleV1,
	}
	key, err := evalpipeline.DescriptorKeyFromRoute(route)
	if err != nil {
		t.Fatal(err)
	}
	family, hit, ok := evalpipeline.ExecutionFamilyFromRouteWithCompat(route)
	if !ok || hit.Used || hit.Source != evalpipeline.CompatibilitySourceFrozen {
		t.Fatalf("family compat = %v %#v", ok, hit)
	}
	if family != modelcatalog.AlgorithmFamilyFactorScoring || key.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("key = %#v", key)
	}
}
