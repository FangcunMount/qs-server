package typology

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// PipelineComponents 是 factor_classification 的原生 RuntimeDescriptor 三件套。
type PipelineComponents struct {
	InputAssembler   evalpipeline.InputAssembler
	Calculator       evalpipeline.Calculator
	OutcomeAssembler evalpipeline.OutcomeAssembler
}

// NewPipelineComponents 构建 factor_classification 原生 descriptor pipeline triple。
func NewPipelineComponents(registry ModuleRegistry) PipelineComponents {
	if registry.Len() == 0 {
		registry = mustDefaultModuleRegistry()
	}
	return PipelineComponents{
		InputAssembler:   typologyInputAssembler{},
		Calculator:       typologyCalculator{registry: registry},
		OutcomeAssembler: typologyPipelineOutcomeAssembler{},
	}
}

type typologyInputAssembler struct{}

func (typologyInputAssembler) Assemble(route evalpipeline.ModelRoute) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Route: route}, nil
}

type typologyCalculator struct {
	registry ModuleRegistry
}

func (c typologyCalculator) Calculate(ctx context.Context, _ evalpipeline.CalculationInput) (any, error) {
	execInput, ok := evaluationexecute.ExecutionInputFromContext(ctx)
	if !ok {
		return nil, evaluationexecute.ErrDescriptorPipelineContext
	}
	if execInput.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if execInput.Input == nil {
		return nil, fmt.Errorf("evaluation input is required")
	}
	payload, ok := port.TypologyPayload(execInput.Input)
	if !ok {
		return nil, fmt.Errorf("personality typology payload is required")
	}
	runner, err := c.registry.runnerForIdentity(evaluation.ExecutionIdentityPersonalityTypology)
	if err != nil {
		return nil, err
	}
	modelRef := modelRefFromExecutionInput(execInput, payload)
	return runner.buildOutcome(modelRef, payload, execInput.Input.AnswerSheet)
}

type typologyPipelineOutcomeAssembler struct{}

func (typologyPipelineOutcomeAssembler) Assemble(result any) (any, error) {
	outcome, ok := result.(*assessment.AssessmentOutcome)
	if !ok || outcome == nil {
		return nil, fmt.Errorf("factor_classification outcome assembler received invalid type %T", result)
	}
	return outcome, nil
}
