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
		{DecisionKind: domain.DecisionKindScoreRange}, {DecisionKind: domain.DecisionKindNormLookup},
		{DecisionKind: domain.DecisionKindAbilityLevel}, {DecisionKind: domain.DecisionKindPoleComposition},
		{DecisionKind: domain.DecisionKindTraitProfile}, {DecisionKind: domain.DecisionKindNearestPattern},
		{DecisionKind: domain.DecisionKindDominantFactor},
	}
	if registry.Len() != len(routes) {
		t.Fatalf("runtime descriptor count = %d, want %d exact routes", registry.Len(), len(routes))
	}
	for _, route := range routes {
		if _, err := registry.Resolve(route); err != nil {
			t.Fatalf("runtime descriptor missing for route %#v: %v", route, err)
		}
	}
	if _, err := registry.Resolve(evalpipeline.ModelRoute{DecisionKind: domain.DecisionKind("unknown")}); err == nil {
		t.Fatal("unknown decision kind must not resolve")
	}
}
