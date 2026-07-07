package execute

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type runtimeEvaluatorStub struct {
	key     evaluation.EvaluatorKey
	execute func(context.Context, ExecutionInput) (*assessment.AssessmentOutcome, error)
}

func (e runtimeEvaluatorStub) Key() evaluation.EvaluatorKey { return e.key }

func (e runtimeEvaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e.execute != nil {
		return e.execute(ctx, input)
	}
	return assessment.NewAssessmentOutcome(assessment.EvaluationModelRef{}, assessment.ResultSummary{}, assessment.EvaluationDetail{}), nil
}

func TestRuntimeResolverUsesDescriptorPrimaryPath(t *testing.T) {
	t.Parallel()

	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	if err := registry.Register(evalpipeline.RuntimeDescriptor{
		Key:             evalpipeline.RuntimeDescriptorKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		ExecutionPath:   modelcatalog.ExecutionPathScaleDescriptor,
	}); err != nil {
		t.Fatal(err)
	}
	evaluatorRegistry, err := NewEvaluatorRegistry(runtimeEvaluatorStub{key: evaluation.EvaluatorKeyScaleDefault})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewRuntimeResolver(registry, evaluatorRegistry, map[modelcatalog.AlgorithmFamily]Evaluator{
		modelcatalog.AlgorithmFamilyFactorScoring: runtimeEvaluatorStub{key: evaluation.EvaluatorKeyScaleDefault},
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

	evaluatorRegistry, err := NewEvaluatorRegistry(runtimeEvaluatorStub{key: evaluation.EvaluatorKeyScaleDefault})
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
	if resolved.EvaluatorKey != evaluation.EvaluatorKeyScaleDefault {
		t.Fatalf("key=%s", resolved.EvaluatorKey)
	}
}
