package pipeline_test

import (
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionRoutingFromSnapshotUsesKindPrimaryFamilies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		snapshot modelcatalog.PublishedModelSnapshot
		family   modelcatalog.AlgorithmFamily
		decision modelcatalog.DecisionKind
	}{
		{
			name: "behavioral_rating_default",
			snapshot: modelcatalog.PublishedModelSnapshot{
				Model: modelcatalog.ModelDefinition{
					Kind:      modelcatalog.KindBehavioralRating,
					Algorithm: modelcatalog.AlgorithmBehavioralRatingDefault,
				},
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange},
			},
			family:   modelcatalog.AlgorithmFamilyFactorNorm,
			decision: modelcatalog.DecisionKindNormLookup,
		},
		{
			name: "cognitive_default",
			snapshot: modelcatalog.PublishedModelSnapshot{
				Model: modelcatalog.ModelDefinition{
					Kind:      modelcatalog.KindCognitive,
					Algorithm: modelcatalog.AlgorithmSPM,
				},
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange},
			},
			family:   modelcatalog.AlgorithmFamilyTaskPerformance,
			decision: modelcatalog.DecisionKindAbilityLevel,
		},
		{
			name: "typology_mbti",
			snapshot: modelcatalog.PublishedModelSnapshot{
				Model: modelcatalog.ModelDefinition{
					Kind:      modelcatalog.KindTypology,
					SubKind:   modelcatalog.SubKindTypology,
					Algorithm: modelcatalog.AlgorithmMBTI,
				},
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
			},
			family:   modelcatalog.AlgorithmFamilyFactorClassification,
			decision: modelcatalog.DecisionKindPoleComposition,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, err := evalpipeline.ExecutionRoutingFromSnapshot(tc.snapshot)
			if err != nil {
				t.Fatalf("ExecutionRoutingFromSnapshot: %v", err)
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
