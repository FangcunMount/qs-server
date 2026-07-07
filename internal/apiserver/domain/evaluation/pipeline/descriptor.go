package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RuntimeDescriptorKey routes evaluation execution by mechanism, not assessment code.
type RuntimeDescriptorKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	PayloadFormat   string
}

func (k RuntimeDescriptorKey) String() string {
	if k.PayloadFormat == "" {
		return k.AlgorithmFamily.String()
	}
	return k.AlgorithmFamily.String() + "/" + k.PayloadFormat
}

// CalculationInput is the mechanism-neutral input passed into a calculator.
type CalculationInput struct {
	Snapshot modelcatalog.PublishedModelSnapshot
}

// Calculator runs the calculation stage for a published model snapshot.
type Calculator interface {
	Calculate(ctx context.Context, input CalculationInput) (any, error)
}

// InputAssembler adapts a published snapshot into calculation input.
type InputAssembler interface {
	Assemble(snapshot modelcatalog.PublishedModelSnapshot) (CalculationInput, error)
}

// OutcomeAssembler adapts calculation output into the canonical assessment outcome.
type OutcomeAssembler interface {
	Assemble(result any) (any, error)
}

// RuntimeDescriptor binds mechanism identity to execution collaborators.
type RuntimeDescriptor struct {
	Key              RuntimeDescriptorKey
	AlgorithmFamily  modelcatalog.AlgorithmFamily
	PayloadFormat    string
	DecisionKind     modelcatalog.DecisionKind
	ExecutionPath    modelcatalog.ExecutionPath
	InputAssembler   InputAssembler
	Calculator       Calculator
	OutcomeAssembler OutcomeAssembler
}

// EvaluationPipeline executes one evaluation for a published model snapshot.
type EvaluationPipeline interface {
	Supports(snapshot modelcatalog.PublishedModelSnapshot) bool
	Execute(ctx context.Context, snapshot modelcatalog.PublishedModelSnapshot) (any, error)
}
