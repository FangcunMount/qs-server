package typology

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/inputinvariant"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// PipelineComponents 是 factor_classification 的原生 RuntimeDescriptor 三件套。
type PipelineComponents struct {
	InputAssembler   evalpipeline.InputAssembler
	Calculator       evalpipeline.Calculator
	OutcomeAssembler evalpipeline.OutcomeAssembler
}

// NewPipelineComponents 构建 factor_classification 原生 descriptor pipeline triple。
func NewPipelineComponents() PipelineComponents {
	return NewPipelineComponentsWithRuntime(DefaultPersonalityRuntime())
}

func NewPipelineComponentsWithRuntime(runtime PersonalityRuntime) PipelineComponents {
	return PipelineComponents{
		InputAssembler:   typologyInputAssembler{},
		Calculator:       typologyCalculator{runtime: runtime},
		OutcomeAssembler: typologyPipelineOutcomeAssembler{},
	}
}

type typologyInputAssembler struct{}

func (typologyInputAssembler) Assemble(route evalpipeline.ModelRoute) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Route: route}, nil
}

type typologyCalculator struct {
	runtime PersonalityRuntime
}

func (c typologyCalculator) Calculate(ctx context.Context, _ evalpipeline.CalculationInput) (any, error) {
	execInput, ok := evalpipeline.ExecutionInputFromContext(ctx)
	if !ok {
		return nil, evalpipeline.ErrExecutionContextMissing
	}
	if err := inputinvariant.Validate(inputinvariant.Input{
		Assessment:    execInput.Assessment,
		Snapshot:      execInput.Input,
		DescriptorKey: "factor_classification",
	}); err != nil {
		return nil, err
	}
	payload, ok := port.TypologyPayload(execInput.Input)
	if !ok {
		return nil, fmt.Errorf("personality typology payload is required")
	}
	runner, err := c.runtime.runnerForIdentity(evaluation.ExecutionIdentityPersonalityTypology)
	if err != nil {
		return nil, err
	}
	modelRef := modelRefFromExecutionInput(execInput, payload)
	return runner.buildOutcome(modelRef, execInput.Input, payload, execInput.Input.AnswerSheet)
}

type typologyPipelineOutcomeAssembler struct{}

func (typologyPipelineOutcomeAssembler) Assemble(result any) (any, error) {
	outcome, ok := result.(*domainoutcome.Execution)
	if !ok || outcome == nil {
		return nil, fmt.Errorf("factor_classification outcome assembler received invalid type %T", result)
	}
	return outcome, nil
}
