package evaluation_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestEvaluationModuleRegistersOnlyDeclaredDescriptorFamilies(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry() error = %v", err)
	}
	routes := []evalpipeline.ModelRoute{
		{AlgorithmFamily: domain.AlgorithmFamilyFactorScoring, DecisionKind: domain.DecisionKindScoreRange},
		{AlgorithmFamily: domain.AlgorithmFamilyFactorNorm, DecisionKind: domain.DecisionKindNormLookup},
		{AlgorithmFamily: domain.AlgorithmFamilyTaskPerformance, DecisionKind: domain.DecisionKindAbilityLevel},
		{AlgorithmFamily: domain.AlgorithmFamilyFactorClassification, DecisionKind: domain.DecisionKindPoleComposition},
		{AlgorithmFamily: domain.AlgorithmFamilyFactorClassification, DecisionKind: domain.DecisionKindTraitProfile},
		{AlgorithmFamily: domain.AlgorithmFamilyFactorClassification, DecisionKind: domain.DecisionKindNearestPattern},
		{AlgorithmFamily: domain.AlgorithmFamilyFactorClassification, DecisionKind: domain.DecisionKindDominantFactor},
	}
	if registry.Len() != len(routes) {
		t.Fatalf("runtime descriptor count = %d, want %d exact routes", registry.Len(), len(routes))
	}
	for _, route := range routes {
		if _, err := registry.Resolve(route); err != nil {
			t.Fatalf("runtime descriptor missing for route %#v: %v", route, err)
		}
	}
	if _, err := registry.Resolve(evalpipeline.ModelRoute{
		AlgorithmFamily: domain.AlgorithmFamilyFactorClassification,
		DecisionKind:    domain.DecisionKindScoreRange,
	}); err == nil {
		t.Fatal("conflicting family and decision must not resolve through fallback")
	}
}
