package execute

import (
	"context"
	"errors"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

var ErrDescriptorPipelineContext = errors.New("descriptor pipeline execution context is missing")

type executionInputContextKey struct{}

// ContextWithExecutionInput attaches execute-time input for descriptor calculators.
func ContextWithExecutionInput(ctx context.Context, input ExecutionInput) context.Context {
	return context.WithValue(ctx, executionInputContextKey{}, input)
}

func executionInputFromContext(ctx context.Context) (ExecutionInput, bool) {
	input, ok := ctx.Value(executionInputContextKey{}).(ExecutionInput)
	return input, ok
}

type snapshotInputAssembler struct{}

func (snapshotInputAssembler) Assemble(snapshot modelcatalog.PublishedModelSnapshot) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Snapshot: snapshot}, nil
}

type evaluatorCalculator struct {
	evaluator Evaluator
}

func (c evaluatorCalculator) Calculate(ctx context.Context, _ evalpipeline.CalculationInput) (any, error) {
	input, ok := executionInputFromContext(ctx)
	if !ok || c.evaluator == nil {
		return nil, ErrDescriptorPipelineContext
	}
	return c.evaluator.Execute(ctx, input)
}

type passThroughOutcomeAssembler struct{}

func (passThroughOutcomeAssembler) Assemble(result any) (any, error) {
	return result, nil
}

// EvaluatorPipelineComponents wraps a family evaluator as descriptor pipeline triple.
func EvaluatorPipelineComponents(evaluator Evaluator) (
	evalpipeline.InputAssembler,
	evalpipeline.Calculator,
	evalpipeline.OutcomeAssembler,
) {
	return snapshotInputAssembler{}, evaluatorCalculator{evaluator: evaluator}, passThroughOutcomeAssembler{}
}
