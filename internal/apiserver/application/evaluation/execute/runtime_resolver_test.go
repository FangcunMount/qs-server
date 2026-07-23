package execute

import (
	"context"
	"testing"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestRuntimeResolverUsesDescriptorPrimaryPath(t *testing.T) {
	t.Parallel()

	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	if err := registry.Register(evalpipeline.RuntimeDescriptor{
		Key:                evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange},
		AlgorithmFamily:    modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:      modelcatalog.ExecutionPathScaleDescriptor,
		CompletenessPolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange),
		InputAssembler:     runtimeStubInputAssembler{},
		Calculator:         runtimeStubCalculator{},
		OutcomeAssembler:   runtimeStubOutcomeAssembler{},
	}); err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(registry, descriptorDrivenExecutor{})

	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind: evaluationinput.EvaluationModelKindScale, Algorithm: "scale_default", Code: "PHQ9",
			DecisionKind: string(modelcatalog.DecisionKindScoreRange),
		},
	}
	outcome, resolved, err := resolver.Execute(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if outcome == nil {
		t.Fatal("outcome is nil")
	}
	if resolved.Descriptor.ExecutionPath != modelcatalog.ExecutionPathScaleDescriptor {
		t.Fatalf("path=%s", resolved.Descriptor.ExecutionPath)
	}
}

func TestRuntimeResolverRejectsMissingDescriptorRegistry(t *testing.T) {
	t.Parallel()

	resolver := NewRuntimeResolver(nil, descriptorDrivenExecutor{})

	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: "scale_default",
			Code:      "PHQ9",
		},
	}
	if _, _, err := resolver.Execute(context.Background(), nil, input); err == nil {
		t.Fatal("Execute error = nil, want missing descriptor registry error")
	}
}

func TestRuntimeResolverRejectsFrozenRouteWithoutFallingBackToAssessment(t *testing.T) {
	t.Parallel()

	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	if err := registry.Register(evalpipeline.RuntimeDescriptor{
		Key:                evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange},
		AlgorithmFamily:    modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:      modelcatalog.ExecutionPathScaleDescriptor,
		CompletenessPolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange),
		InputAssembler:     runtimeStubInputAssembler{},
		Calculator:         runtimeStubCalculator{},
		OutcomeAssembler:   runtimeStubOutcomeAssembler{},
	}); err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(registry, descriptorDrivenExecutor{})

	// Frozen behavioral/norm route is not registered; Assessment ModelRef is scale and WOULD match the registry.
	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:         evaluationinput.EvaluationModelKindBehavioralRating,
			Algorithm:    string(modelcatalog.AlgorithmBrief2),
			DecisionKind: string(modelcatalog.DecisionKindNormLookup),
			Code:         "BR-001",
			Version:      "1.0.0",
		},
	}
	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(9001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(1)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithEvaluationModel(domainAssessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("PHQ9"), "1.0.0", "PHQ9")),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = resolver.Execute(context.Background(), a, input)
	if err == nil {
		t.Fatal("expected frozen route failure without Assessment ModelRef fallback to scale descriptor")
	}
}

func TestRuntimeResolverRejectsIncompleteInput(t *testing.T) {
	t.Parallel()

	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	if err := registry.Register(evalpipeline.RuntimeDescriptor{
		Key:                evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange},
		AlgorithmFamily:    modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:      modelcatalog.ExecutionPathScaleDescriptor,
		CompletenessPolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange),
		InputAssembler:     runtimeStubInputAssembler{},
		Calculator:         runtimeStubCalculator{},
		OutcomeAssembler:   runtimeStubOutcomeAssembler{},
	}); err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(registry, descriptorDrivenExecutor{})
	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: "scale_default",
			Code:      "PHQ9",
		},
	}
	if _, _, err := resolver.Execute(context.Background(), nil, input); err == nil {
		t.Fatal("incomplete input route was accepted")
	}
}

type runtimeStubInputAssembler struct{}

func (runtimeStubInputAssembler) Assemble(input evalpipeline.ExecutionInput) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Execution: input}, nil
}

type runtimeStubCalculator struct{}

func (runtimeStubCalculator) Calculate(context.Context, evalpipeline.CalculationInput) (any, error) {
	return struct{}{}, nil
}

type runtimeStubOutcomeAssembler struct{}

func (runtimeStubOutcomeAssembler) Assemble(any) (any, error) {
	return domainoutcome.NewExecution(domainoutcome.ModelRef{}, domainoutcome.Summary{}, domainoutcome.Detail{}), nil
}
