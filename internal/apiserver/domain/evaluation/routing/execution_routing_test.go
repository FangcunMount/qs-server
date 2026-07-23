package evaluation_test

import (
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDescriptorKeyFromRouteUsesFrozenRuntimeIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		route    evalpipeline.ModelRoute
		family   modelcatalog.AlgorithmFamily
		decision modelcatalog.DecisionKind
	}{
		{
			name: "behavioral_rating_default",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindNormLookup},
			family:   modelcatalog.AlgorithmFamilyFactorNorm,
			decision: modelcatalog.DecisionKindNormLookup,
		},
		{
			name: "cognitive_default",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindAbilityLevel},
			family:   modelcatalog.AlgorithmFamilyTaskPerformance,
			decision: modelcatalog.DecisionKindAbilityLevel,
		},
		{
			name: "typology_pole_composition",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindPoleComposition},
			family:   modelcatalog.AlgorithmFamilyFactorClassification,
			decision: modelcatalog.DecisionKindPoleComposition,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, err := evalpipeline.DescriptorKeyFromRoute(tc.route)
			if err != nil {
				t.Fatalf("DescriptorKeyFromRoute: %v", err)
			}
			family, ok := evalpipeline.ExecutionFamilyFromRoute(tc.route)
			if !ok || family != tc.family {
				t.Fatalf("derived family=%s want=%s", family, tc.family)
			}
			if key.DecisionKind != tc.decision {
				t.Fatalf("decision=%s want=%s", key.DecisionKind, tc.decision)
			}
		})
	}
}
