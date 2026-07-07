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
	if key.DecisionKind != modelcatalog.DecisionKindNormLookup {
		t.Fatalf("decision kind=%s want=%s", key.DecisionKind, modelcatalog.DecisionKindNormLookup)
	}
}

func TestRuntimeDescriptorKeyFromSnapshotDifferentiatesDecisionKindWithinFamily(t *testing.T) {
	t.Parallel()

	pole, err := RuntimeDescriptorKeyFromSnapshot(modelcatalog.PublishedModelSnapshot{
		Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
		PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	trait, err := RuntimeDescriptorKeyFromSnapshot(modelcatalog.PublishedModelSnapshot{
		Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
		PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pole.AlgorithmFamily != trait.AlgorithmFamily {
		t.Fatalf("families diverged: pole=%s trait=%s", pole.AlgorithmFamily, trait.AlgorithmFamily)
	}
	if pole.DecisionKind == trait.DecisionKind {
		t.Fatalf("decision kinds should differ within same family: %s", pole.DecisionKind)
	}
	if pole.String() == trait.String() {
		t.Fatalf("key strings should differ: %s", pole.String())
	}
}

func TestRuntimeDescriptorRegistryResolvesByFormatFallback(t *testing.T) {
	t.Parallel()

	registry := NewRuntimeDescriptorRegistry()
	desc := RuntimeDescriptor{
		Key: RuntimeDescriptorKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			PayloadFormat:   modelcatalog.PayloadFormatPersonalityTypologyV1,
		},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		ExecutionPath:   modelcatalog.ExecutionPathTypologyDescriptor,
	}
	if err := registry.Register(desc); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Resolve(modelcatalog.PublishedModelSnapshot{
		Decision:      modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindTraitProfile},
		PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionPath != modelcatalog.ExecutionPathTypologyDescriptor {
		t.Fatalf("path=%s", got.ExecutionPath)
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
