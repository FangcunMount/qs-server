package pipeline

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathForModelKindUsesMechanismFamilies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind ModelKind
		want modelcatalog.ExecutionPath
	}{
		{ModelKindScale, modelcatalog.ExecutionPathScaleDescriptor},
		{ModelKindTypology, modelcatalog.ExecutionPathTypologyDescriptor},
		{ModelKindBehavioralRating, modelcatalog.ExecutionPathBehavioralRatingDescriptor},
		{ModelKindCognitive, modelcatalog.ExecutionPathCognitiveDescriptor},
	}
	for _, tc := range cases {
		path, err := ExecutionPathForModelKind(tc.kind)
		if err != nil {
			t.Fatalf("kind=%s: %v", tc.kind, err)
		}
		if path != tc.want {
			t.Fatalf("kind=%s path=%s want=%s", tc.kind, path, tc.want)
		}
	}
}

func TestRuntimeDescriptorKeyFromSnapshotUsesDecisionKind(t *testing.T) {
	t.Parallel()

	key, err := RuntimeDescriptorKeyFromSnapshot(modelcatalog.PublishedModelSnapshot{
		Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindNormLookup},
		PayloadFormat: modelcatalog.PayloadFormatBehavioralRatingBrief2V1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm {
		t.Fatalf("family=%s want=%s", key.AlgorithmFamily, modelcatalog.AlgorithmFamilyFactorNorm)
	}
	if key.PayloadFormat != modelcatalog.PayloadFormatBehavioralRatingBrief2V1 {
		t.Fatalf("payload format=%s", key.PayloadFormat)
	}
}

func TestRuntimeDescriptorRegistryResolvesByFamilyFallback(t *testing.T) {
	t.Parallel()

	registry := NewRuntimeDescriptorRegistry()
	desc := RuntimeDescriptor{
		Key:             RuntimeDescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:   modelcatalog.ExecutionPathScaleDescriptor,
	}
	if err := registry.Register(desc); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Resolve(modelcatalog.PublishedModelSnapshot{
		Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange},
		PayloadFormat: modelcatalog.PayloadFormatAssessmentScaleV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("family=%s", got.AlgorithmFamily)
	}
}
