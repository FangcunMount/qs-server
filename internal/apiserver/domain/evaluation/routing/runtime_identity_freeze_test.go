package evaluation_test

import (
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFrozenRuntimeIdentityDoesNotSilentFallback(t *testing.T) {
	t.Parallel()

	route := evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKind("unknown")}
	if !route.HasFrozenRuntime() {
		t.Fatal("expected frozen runtime")
	}
	_, err := evalpipeline.DescriptorKeyFromRoute(route)
	if err == nil {
		t.Fatal("expected error for unknown frozen decision")
	}
}

func TestFrozenRuntimeIdentityPreferredOverIdentityDerivation(t *testing.T) {
	t.Parallel()

	route := evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindScoreRange}
	key, err := evalpipeline.DescriptorKeyFromRoute(route)
	if err != nil {
		t.Fatal(err)
	}
	family, ok := evalpipeline.ExecutionFamilyFromRoute(route)
	if !ok {
		t.Fatal("frozen route was not resolved")
	}
	if family != modelcatalog.AlgorithmFamilyFactorScoring || key.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("key = %#v", key)
	}
}
