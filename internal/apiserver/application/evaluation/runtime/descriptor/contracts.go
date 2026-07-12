// Package descriptor owns the application runtime contracts used to execute
// an Evaluation routing decision. Pure identity and routing policy remain in
// domain/evaluation/routing.
package descriptor

import (
	"context"
	"errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type (
	ExecutionIdentity = evalrouting.ExecutionIdentity
	ModelRoute        = evalrouting.ModelRoute
	DescriptorKey     = evalrouting.DescriptorKey
)

var (
	ExecutionIdentityScaleDefault            = evalrouting.ExecutionIdentityScaleDefault
	ExecutionIdentityPersonalityTypology     = evalrouting.ExecutionIdentityPersonalityTypology
	ExecutionIdentityBehavioralRatingDefault = evalrouting.ExecutionIdentityBehavioralRatingDefault
	ExecutionIdentityCognitiveDefault        = evalrouting.ExecutionIdentityCognitiveDefault
)

func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	return evalrouting.DescriptorKeyFromRoute(route)
}

func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	return evalrouting.ExecutionFamilyFromRoute(route)
}

type CalculationInput struct {
	Route ModelRoute
}

type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

type DescriptorExecutor interface {
	Execute(context.Context, RuntimeDescriptor, ExecutionInput) (*domainoutcome.Execution, error)
}

var ErrExecutionContextMissing = errors.New("descriptor execution context is missing")

type executionInputContextKey struct{}

func ContextWithExecutionInput(ctx context.Context, input ExecutionInput) context.Context {
	return context.WithValue(ctx, executionInputContextKey{}, input)
}

func ExecutionInputFromContext(ctx context.Context) (ExecutionInput, bool) {
	input, ok := ctx.Value(executionInputContextKey{}).(ExecutionInput)
	return input, ok
}

type Calculator interface {
	Calculate(context.Context, CalculationInput) (any, error)
}

type InputAssembler interface {
	Assemble(ModelRoute) (CalculationInput, error)
}

type OutcomeAssembler interface {
	Assemble(any) (any, error)
}

type RuntimeDescriptor struct {
	Key              DescriptorKey
	AlgorithmFamily  modelcatalog.AlgorithmFamily
	PayloadFormat    string
	DecisionKind     modelcatalog.DecisionKind
	ExecutionPath    modelcatalog.ExecutionPath
	InputAssembler   InputAssembler
	Calculator       Calculator
	OutcomeAssembler OutcomeAssembler
}
