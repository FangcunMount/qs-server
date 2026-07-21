// Package descriptor owns the application runtime contracts used to execute
// an Evaluation routing decision. Pure identity and routing policy remain in
// domain/evaluation/routing.
package descriptor

import (
	"context"

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
	ExecutionIdentityCognitiveDefault        = evalrouting.ExecutionIdentityCognitiveDefault
)

func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	return evalrouting.DescriptorKeyFromRoute(route)
}

func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	return evalrouting.ExecutionFamilyFromRoute(route)
}

type CalculationInput struct {
	Route     ModelRoute
	Execution ExecutionInput
}

type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

type DescriptorExecutor interface {
	Execute(context.Context, RuntimeDescriptor, ExecutionInput) (*domainoutcome.Execution, error)
}

type Calculator interface {
	Calculate(context.Context, CalculationInput) (any, error)
}

type InputAssembler interface {
	Assemble(ExecutionInput) (CalculationInput, error)
}

type OutcomeAssembler interface {
	Assemble(any) (any, error)
}

type RuntimeDescriptor struct {
	Key              DescriptorKey
	AlgorithmFamily  modelcatalog.AlgorithmFamily
	DecisionKind     modelcatalog.DecisionKind
	ExecutionPath    modelcatalog.ExecutionPath
	InputAssembler   InputAssembler
	Calculator       Calculator
	OutcomeAssembler OutcomeAssembler
}
