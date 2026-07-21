package descriptor

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestRegistryResolvesOnlyExactFamilyDecision(t *testing.T) {
	registry := NewRuntimeDescriptorRegistry()
	desc := RuntimeDescriptor{Key: DescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile}, AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile, ExecutionPath: modelcatalog.ExecutionPathTypologyDescriptor}
	if err := registry.Register(desc); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(ModelRoute{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(ModelRoute{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition}); err == nil {
		t.Fatal("family fallback unexpectedly resolved a different decision")
	}
}

func TestRegistryRejectsIncompleteKey(t *testing.T) {
	registry := NewRuntimeDescriptorRegistry()
	err := registry.Register(RuntimeDescriptor{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring})
	if err == nil {
		t.Fatal("Register accepted descriptor without decision kind")
	}
}
