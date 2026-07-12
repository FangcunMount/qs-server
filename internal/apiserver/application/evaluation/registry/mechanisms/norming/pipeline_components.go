package norming

import (
	"context"
	"fmt"

	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// PipelineComponents 是 factor_norm 的原生 RuntimeDescriptor 三件套。
type PipelineComponents struct {
	InputAssembler   evalpipeline.InputAssembler
	Calculator       evalpipeline.Calculator
	OutcomeAssembler evalpipeline.OutcomeAssembler
}

// NewPipelineComponents 构建 factor_norm 原生 descriptor pipeline triple。
func NewPipelineComponents(scorer ruleengine.ScaleFactorScorer) PipelineComponents {
	return NewPipelineComponentsWithScoring(factorscoring.NewExecutor(scorer))
}

// NewPipelineComponentsWithScoring 构建可注入 scoring executor 的 factor_norm pipeline triple。
func NewPipelineComponentsWithScoring(scoring *factorscoring.Executor) PipelineComponents {
	if scoring == nil {
		scoring = factorscoring.NewExecutor(nil)
	}
	return PipelineComponents{
		InputAssembler:   factorNormInputAssembler{},
		Calculator:       factorNormCalculator{scoring: scoring},
		OutcomeAssembler: factorNormOutcomeAssembler{},
	}
}

type factorNormInputAssembler struct{}

func (factorNormInputAssembler) Assemble(route evalpipeline.ModelRoute) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Route: route}, nil
}

type factorNormCalculator struct {
	scoring *factorscoring.Executor
}

type factorNormPipelineResult struct {
	outcome *domainoutcome.Execution
	input   *portevaluationinput.InputSnapshot
}

func (c factorNormCalculator) Calculate(ctx context.Context, _ evalpipeline.CalculationInput) (any, error) {
	if c.scoring == nil {
		return nil, fmt.Errorf("factor_norm evaluation calculator is not configured")
	}
	execInput, ok := evalpipeline.ExecutionInputFromContext(ctx)
	if !ok {
		return nil, evalpipeline.ErrExecutionContextMissing
	}
	scaleSnapshot, ok := portevaluationinput.BehavioralRatingScaleSnapshot(execInput.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("behavioral_rating model payload is required")
	}
	outcome, err := c.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: execInput.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(execInput.Input, scaleSnapshot),
	})
	if err != nil {
		return nil, err
	}
	return factorNormPipelineResult{
		outcome: outcome,
		input:   execInput.Input,
	}, nil
}

type factorNormOutcomeAssembler struct{}

func (factorNormOutcomeAssembler) Assemble(result any) (any, error) {
	pipelineResult, ok := result.(factorNormPipelineResult)
	if !ok || pipelineResult.outcome == nil {
		return nil, fmt.Errorf("factor_norm outcome assembler received invalid type %T", result)
	}
	payload, ok := portevaluationinput.BehavioralRatingPayload(pipelineResult.input)
	if !ok || payload.Snapshot == nil {
		return pipelineResult.outcome, nil
	}
	return ApplyFactorProjections(pipelineResult.outcome, payload.Snapshot, NormSubjectFromInput(pipelineResult.input)), nil
}
