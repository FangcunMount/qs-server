package descriptor

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestRegistryResolutionFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("payload format", func(t *testing.T) {
		registry := NewRuntimeDescriptorRegistry()
		desc := RuntimeDescriptor{
			Key: DescriptorKey{
				AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
				PayloadFormat:   modelcatalog.PayloadFormatPersonalityTypologyV1,
			},
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			ExecutionPath:   modelcatalog.ExecutionPathTypologyDescriptor,
		}
		if err := registry.Register(desc); err != nil {
			t.Fatal(err)
		}
		got, err := registry.Resolve(ModelRoute{
			DecisionKind:  modelcatalog.DecisionKindTraitProfile,
			PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.ExecutionPath != modelcatalog.ExecutionPathTypologyDescriptor {
			t.Fatalf("path=%s", got.ExecutionPath)
		}
	})

	t.Run("explicit family", func(t *testing.T) {
		registry := NewRuntimeDescriptorRegistry()
		desc := RuntimeDescriptor{
			Key:             DescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
			ExecutionPath:   modelcatalog.ExecutionPathScaleDescriptor,
		}
		if err := registry.Register(desc); err != nil {
			t.Fatal(err)
		}
		got, err := registry.Resolve(ModelRoute{
			DecisionKind:  modelcatalog.DecisionKindScoreRange,
			PayloadFormat: modelcatalog.PayloadFormatAssessmentScaleV1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
			t.Fatalf("family=%s", got.AlgorithmFamily)
		}
	})
}

func TestRegistryRequiresExplicitFamilyFallback(t *testing.T) {
	t.Parallel()

	registry := NewRuntimeDescriptorRegistry()
	desc := RuntimeDescriptor{
		Key: DescriptorKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			PayloadFormat:   modelcatalog.PayloadFormatPersonalityTypologyV1,
		},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		ExecutionPath:   modelcatalog.ExecutionPathTypologyDescriptor,
	}
	if err := registry.Register(desc); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Resolve(ModelRoute{
		DecisionKind:  modelcatalog.DecisionKindTraitProfile,
		PayloadFormat: "assessmentmodel.personality.trait-profile.v2",
	})
	if err == nil {
		t.Fatal("Resolve error = nil, want unsupported descriptor")
	}
	if registry.HasAlgorithmFamily(modelcatalog.AlgorithmFamilyFactorClassification) {
		t.Fatal("format-specific descriptor must not become a family fallback")
	}
}
