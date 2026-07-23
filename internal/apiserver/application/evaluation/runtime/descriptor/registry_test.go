package descriptor

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestRegistryResolvesOnlyExactFamilyDecision(t *testing.T) {
	registry := NewRuntimeDescriptorRegistry()
	desc := RuntimeDescriptor{
		Key: DescriptorKey{DecisionKind: modelcatalog.DecisionKindTraitProfile}, AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		DecisionKind: modelcatalog.DecisionKindTraitProfile, ExecutionPath: modelcatalog.ExecutionPathTypologyDescriptor,
		CompletenessPolicy: DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindTraitProfile),
	}
	if err := registry.Register(desc); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(ModelRoute{DecisionKind: modelcatalog.DecisionKindTraitProfile}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(ModelRoute{DecisionKind: modelcatalog.DecisionKindPoleComposition}); err == nil {
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

func TestRegistryRejectsMissingOutcomeCompletenessPolicy(t *testing.T) {
	registry := NewRuntimeDescriptorRegistry()
	err := registry.Register(RuntimeDescriptor{
		Key: DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange}, AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
	})
	if err == nil {
		t.Fatal("Register accepted descriptor without outcome completeness policy")
	}
}
