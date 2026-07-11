package execute

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type runtimeEvaluatorStub struct {
	key     evaluation.ExecutionIdentity
	execute func(context.Context, ExecutionInput) (*assessment.AssessmentOutcome, error)
}

func (e runtimeEvaluatorStub) ExecutionIdentity() evaluation.ExecutionIdentity { return e.key }

func (e runtimeEvaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
	if e.execute != nil {
		legacy, err := e.execute(ctx, input)
		return evaloutcome.ExecutionFromAssessmentOutcome(legacy), err
	}
	return evaloutcome.ExecutionFromAssessmentOutcome(assessment.NewAssessmentOutcome(assessment.EvaluationModelRef{}, assessment.ResultSummary{}, assessment.EvaluationDetail{})), nil
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
	evaluatorRegistry, err := NewEvaluatorRegistry(runtimeEvaluatorStub{key: evaluation.ExecutionIdentityScaleDefault})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(registry, evaluatorRegistry, map[modelcatalog.AlgorithmFamily]Evaluator{
		modelcatalog.AlgorithmFamilyFactorScoring: runtimeEvaluatorStub{key: evaluation.ExecutionIdentityScaleDefault},
	})

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
	if !resolved.UsedDescriptor {
		t.Fatal("expected descriptor-primary path")
	}
	if resolved.Descriptor.ExecutionPath != modelcatalog.ExecutionPathScaleDescriptor {
		t.Fatalf("path=%s", resolved.Descriptor.ExecutionPath)
	}
}

func TestRuntimeResolverFallsBackToEvaluatorKey(t *testing.T) {
	t.Parallel()

	evaluatorRegistry, err := NewEvaluatorRegistry(runtimeEvaluatorStub{key: evaluation.ExecutionIdentityScaleDefault})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(nil, evaluatorRegistry, nil)

	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: "scale_default",
			Code:      "PHQ9",
		},
	}
	_, resolved, err := resolver.Execute(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.UsedDescriptor {
		t.Fatal("expected legacy fallback without descriptor registry")
	}
	if resolved.ExecutionIdentity != evaluation.ExecutionIdentityScaleDefault {
		t.Fatalf("key=%s", resolved.ExecutionIdentity)
	}
}

func TestRuntimeResolverReturnsDescriptorErrorWhenRegistryCannotResolveSnapshot(t *testing.T) {
	t.Parallel()

	evaluatorRegistry, err := NewEvaluatorRegistry(runtimeEvaluatorStub{key: evaluation.ExecutionIdentityScaleDefault})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(evalpipeline.NewRuntimeDescriptorRegistry(), evaluatorRegistry, nil)

	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: "scale_default",
			Code:      "PHQ9",
		},
	}
	_, _, err = resolver.Execute(context.Background(), nil, input)
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
	return assessment.NewAssessmentOutcome(assessment.EvaluationModelRef{}, assessment.ResultSummary{}, assessment.EvaluationDetail{}), nil
}
