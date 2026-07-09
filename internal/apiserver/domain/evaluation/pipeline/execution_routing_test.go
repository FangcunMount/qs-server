package pipeline_test

import (
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionRoutingFromRouteUsesKindPrimaryFamilies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		route    evalpipeline.ModelRoute
		family   modelcatalog.AlgorithmFamily
		decision modelcatalog.DecisionKind
	}{
		{
			name: "behavioral_rating_default",
			route: evalpipeline.ModelRoute{
				Kind:         modelcatalog.KindBehavioralRating,
				Algorithm:    modelcatalog.AlgorithmBehavioralRatingDefault,
				DecisionKind: modelcatalog.DecisionKindScoreRange,
			},
			family:   modelcatalog.AlgorithmFamilyFactorNorm,
			decision: modelcatalog.DecisionKindNormLookup,
		},
		{
			name: "cognitive_default",
			route: evalpipeline.ModelRoute{
				Kind:         modelcatalog.KindCognitive,
				Algorithm:    modelcatalog.AlgorithmSPM,
				DecisionKind: modelcatalog.DecisionKindScoreRange,
			},
			family:   modelcatalog.AlgorithmFamilyTaskPerformance,
			decision: modelcatalog.DecisionKindAbilityLevel,
		},
		{
			name: "typology_mbti",
			route: evalpipeline.ModelRoute{
				Kind:         modelcatalog.KindTypology,
				SubKind:      modelcatalog.SubKindTypology,
				Algorithm:    modelcatalog.AlgorithmMBTI,
				DecisionKind: modelcatalog.DecisionKindPoleComposition,
			},
			family:   modelcatalog.AlgorithmFamilyFactorClassification,
			decision: modelcatalog.DecisionKindPoleComposition,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, err := evalpipeline.ExecutionRoutingFromRoute(tc.route)
			if err != nil {
				t.Fatalf("ExecutionRoutingFromRoute: %v", err)
			}
			if key.AlgorithmFamily != tc.family {
				t.Fatalf("family=%s want=%s", key.AlgorithmFamily, tc.family)
			}
			if key.DecisionKind != tc.decision {
				t.Fatalf("decision=%s want=%s", key.DecisionKind, tc.decision)
			}
		})
	}
}
