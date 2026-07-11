package execute

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type runtimeEvaluatorStub struct {
	key     evaluation.ExecutionIdentity
	execute func(context.Context, ExecutionInput) (*domainoutcome.Execution, error)
}

func (e runtimeEvaluatorStub) ExecutionIdentity() evaluation.ExecutionIdentity { return e.key }

func (e runtimeEvaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
	if e.execute != nil {
		return e.execute(ctx, input)
	}
	return domainoutcome.NewExecution(domainoutcome.ModelRef{}, domainoutcome.Summary{}, domainoutcome.Detail{}), nil
}

func (e runtimeEvaluatorStub) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}

func TestRuntimeResolverUsesDescriptorPrimaryPath(t *testing.T) {
	t.Parallel()

	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	if err := registry.Register(evalpipeline.RuntimeDescriptor{
		Key:              evalpipeline.RuntimeDescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
		AlgorithmFamily:  modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:    modelcatalog.ExecutionPathScaleDescriptor,
		InputAssembler:   runtimeStubInputAssembler{},
		Calculator:       runtimeStubCalculator{},
		OutcomeAssembler: runtimeStubOutcomeAssembler{},
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

func TestRuntimeResolverReturnsDescriptorErrorWhenRegistryCannotResolveSnapshot(t *testing.T) {
	t.Parallel()

	resolver := NewRuntimeResolver(evalpipeline.NewRuntimeDescriptorRegistry(), descriptorDrivenExecutor{})

	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: "scale_default",
			Code:      "PHQ9",
		},
	}
	_, _, err := resolver.Execute(context.Background(), nil, input)
	if err == nil {
		t.Fatal("Execute error = nil, want descriptor resolve error when registry is configured")
	}
}

type runtimeStubInputAssembler struct{}

func (runtimeStubInputAssembler) Assemble(route evalpipeline.ModelRoute) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Route: route}, nil
}

type runtimeStubCalculator struct{}

func (runtimeStubCalculator) Calculate(context.Context, evalpipeline.CalculationInput) (any, error) {
	return struct{}{}, nil
}

type runtimeStubOutcomeAssembler struct{}

func (runtimeStubOutcomeAssembler) Assemble(any) (any, error) {
	return domainoutcome.NewExecution(domainoutcome.ModelRef{}, domainoutcome.Summary{}, domainoutcome.Detail{}), nil
}
