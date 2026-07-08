package pipeline_test

import (
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathRoutingEquivalenceForModelKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind evaldomain.ModelKind
		want modelcatalog.ExecutionPath
	}{
		{evaldomain.ModelKindScale, modelcatalog.ExecutionPathScaleDescriptor},
		{evaldomain.ModelKindTypology, modelcatalog.ExecutionPathTypologyDescriptor},
		{evaldomain.ModelKindBehavioralRating, modelcatalog.ExecutionPathBehavioralRatingDescriptor},
		{evaldomain.ModelKindCognitive, modelcatalog.ExecutionPathCognitiveDescriptor},
	}
	for _, tc := range cases {
		desc := evaldomain.ModelDescriptor{Kind: tc.kind}
		fromDescriptor, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			t.Fatalf("kind=%s ExecutionPathForDescriptor: %v", tc.kind, err)
		}
		fromPipeline, err := evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(tc.kind))
		if err != nil {
			t.Fatalf("kind=%s ExecutionPathForModelKind: %v", tc.kind, err)
		}
		if fromDescriptor != tc.want {
			t.Fatalf("kind=%s descriptor path=%s want=%s", tc.kind, fromDescriptor, tc.want)
		}
		if fromPipeline != tc.want {
			t.Fatalf("kind=%s pipeline path=%s want=%s", tc.kind, fromPipeline, tc.want)
		}
		if fromDescriptor != fromPipeline {
			t.Fatalf("kind=%s paths diverged: descriptor=%s pipeline=%s", tc.kind, fromDescriptor, fromPipeline)
		}
	}
}

func TestRuntimeDescriptorKeyMatchesIdentityDerivation(t *testing.T) {
	t.Parallel()

	cases := []modelcatalog.PublishedModelSnapshot{
		{
			Model: modelcatalog.ModelDefinition{
				Kind:      modelcatalog.KindScale,
				Algorithm: modelcatalog.AlgorithmScaleDefault,
			},
			Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange},
			PayloadFormat: modelcatalog.PayloadFormatAssessmentScaleV1,
		},
		{
			Model: modelcatalog.ModelDefinition{
				Kind:      modelcatalog.KindTypology,
				SubKind:   modelcatalog.SubKindTypology,
				Algorithm: modelcatalog.AlgorithmMBTI,
			},
			Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
			PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
		},
		{
			Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindNormLookup},
			PayloadFormat: modelcatalog.PayloadFormatBehavioralRatingBrief2V1,
		},
	}
	for i, snapshot := range cases {
		key, err := evalpipeline.RuntimeDescriptorKeyFromSnapshot(snapshot)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		family, ok := evalpipeline.AlgorithmFamilyFromSnapshot(snapshot)
		if !ok {
			t.Fatalf("case %d: AlgorithmFamilyFromSnapshot failed", i)
		}
		if key.AlgorithmFamily != family {
			t.Fatalf("case %d: key family=%s snapshot family=%s", i, key.AlgorithmFamily, family)
		}
		if snapshot.Decision.Kind != "" {
			wantFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(snapshot.Decision.Kind)
			if !ok {
				t.Fatalf("case %d: decision kind %s", i, snapshot.Decision.Kind)
			}
			if key.AlgorithmFamily != wantFamily {
				t.Fatalf("case %d: family=%s want=%s", i, key.AlgorithmFamily, wantFamily)
			}
			if key.DecisionKind != snapshot.Decision.Kind {
				t.Fatalf("case %d: decision=%s want=%s", i, key.DecisionKind, snapshot.Decision.Kind)
			}
		}
	}
}

func TestRuntimeDescriptorKeyAlignsWithMechanismReportBuilderFamilyAndDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		snapshot modelcatalog.PublishedModelSnapshot
	}{
		{
			snapshot: modelcatalog.PublishedModelSnapshot{
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange},
			},
		},
		{
			snapshot: modelcatalog.PublishedModelSnapshot{
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
			},
		},
		{
			snapshot: modelcatalog.PublishedModelSnapshot{
				Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
			},
		},
	}
	for i, tc := range cases {
		key, err := evalpipeline.RuntimeDescriptorKeyFromSnapshot(tc.snapshot)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(tc.snapshot.Decision.Kind)
		if !ok {
			t.Fatalf("case %d: decision %s", i, tc.snapshot.Decision.Kind)
		}
		if key.AlgorithmFamily != family {
			t.Fatalf("case %d: family=%s want=%s", i, key.AlgorithmFamily, family)
		}
		if key.DecisionKind != tc.snapshot.Decision.Kind {
			t.Fatalf("case %d: decision=%s want=%s", i, key.DecisionKind, tc.snapshot.Decision.Kind)
		}
	}
}
