package scoring

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// PipelineComponents 是 factor_scoring 的原生 RuntimeDescriptor 三件套。
type PipelineComponents struct {
	InputAssembler   evalpipeline.InputAssembler
	Calculator       evalpipeline.Calculator
	OutcomeAssembler evalpipeline.OutcomeAssembler
}

// NewPipelineComponents 构建 factor_scoring 原生 descriptor pipeline triple。
func NewPipelineComponents(scorer ruleengine.ScaleFactorScorer) PipelineComponents {
	return NewPipelineComponentsWithDeps(
		DefaultInputValidator{},
		calcscoring.NewEvaluator(scoringRegistry{scorer: scorer}),
	)
}

// NewPipelineComponentsWithDeps 构建可注入依赖的 factor_scoring pipeline triple（测试用）。
func NewPipelineComponentsWithDeps(validator InputValidator, evaluator *calcscoring.Evaluator) PipelineComponents {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if evaluator == nil {
		evaluator = calcscoring.NewDefaultEvaluator()
	}
	return PipelineComponents{
		InputAssembler:   factorScoringInputAssembler{},
		Calculator:       factorScoringCalculator{validator: validator, evaluator: evaluator},
		OutcomeAssembler: factorScoringOutcomeAssembler{},
	}
}

type factorScoringInputAssembler struct{}

func (factorScoringInputAssembler) Assemble(snapshot modelcatalog.PublishedModelSnapshot) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Snapshot: snapshot}, nil
}

type factorScoringCalculator struct {
	validator InputValidator
	evaluator *calcscoring.Evaluator
}

type factorScoringPipelineResult struct {
	result     *calcscoring.Result
	assessment *assessment.Assessment
	snapshot   *evaluationinput.InputSnapshot
}

func (c factorScoringCalculator) Calculate(ctx context.Context, calcInput evalpipeline.CalculationInput) (any, error) {
	if c.evaluator == nil {
		return nil, fmt.Errorf("factor_scoring evaluation calculator is not configured")
	}
	execInput, ok := evaluationexecute.ExecutionInputFromContext(ctx)
	if !ok {
		return nil, evaluationexecute.ErrDescriptorPipelineContext
	}
	scoringInput := ExecutionInput{
		Assessment: execInput.Assessment,
		Input:      execInput.Input,
	}
	if err := c.validator.Validate(scoringInput); err != nil {
		return nil, err
	}
	result, err := c.evaluator.Score(ctx, calcInputFromSnapshot(execInput.Input))
	if err != nil {
		return nil, err
	}
	return factorScoringPipelineResult{
		result:     result,
		assessment: execInput.Assessment,
		snapshot:   execInput.Input,
	}, nil
}

type factorScoringOutcomeAssembler struct{}

func (factorScoringOutcomeAssembler) Assemble(result any) (any, error) {
	pipelineResult, ok := result.(factorScoringPipelineResult)
	if !ok || pipelineResult.result == nil {
		return nil, fmt.Errorf("factor_scoring outcome assembler received invalid type %T", result)
	}
	return ToAssessmentOutcome(pipelineResult.result, pipelineResult.assessment, pipelineResult.snapshot), nil
}
