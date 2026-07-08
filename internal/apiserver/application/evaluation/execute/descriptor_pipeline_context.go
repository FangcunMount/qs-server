package execute

import (
	"context"
	"errors"
)

var ErrDescriptorPipelineContext = errors.New("descriptor pipeline execution context is missing")

type executionInputContextKey struct{}

// ContextWithExecutionInput attaches execute-time input for descriptor calculators.
func ContextWithExecutionInput(ctx context.Context, input ExecutionInput) context.Context {
	return context.WithValue(ctx, executionInputContextKey{}, input)
}

// ExecutionInputFromContext reads execute-time input attached by ContextWithExecutionInput.
func ExecutionInputFromContext(ctx context.Context) (ExecutionInput, bool) {
	input, ok := ctx.Value(executionInputContextKey{}).(ExecutionInput)
	return input, ok
}
