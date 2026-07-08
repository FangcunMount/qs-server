package task_performance

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// PipelineComponents 是 task_performance 的原生 RuntimeDescriptor 三件套。
type PipelineComponents struct {
	InputAssembler   evalpipeline.InputAssembler
	Calculator       evalpipeline.Calculator
	OutcomeAssembler evalpipeline.OutcomeAssembler
}

// NewPipelineComponents 构建 task_performance 原生 descriptor pipeline triple。
func NewPipelineComponents(scorer ruleengine.ScaleFactorScorer) PipelineComponents {
	return NewPipelineComponentsWithScoring(factorscoring.NewExecutor(scorer))
}

// NewPipelineComponentsWithScoring 构建可注入 scoring executor 的 task_performance pipeline triple。
func NewPipelineComponentsWithScoring(scoring *factorscoring.Executor) PipelineComponents {
	if scoring == nil {
		scoring = factorscoring.NewExecutor(nil)
	}
	return PipelineComponents{
		InputAssembler:   taskPerformanceInputAssembler{},
		Calculator:       taskPerformanceCalculator{scoring: scoring},
		OutcomeAssembler: taskPerformanceOutcomeAssembler{},
	}
}

type taskPerformanceInputAssembler struct{}

func (taskPerformanceInputAssembler) Assemble(snapshot modelcatalog.PublishedModelSnapshot) (evalpipeline.CalculationInput, error) {
	return evalpipeline.CalculationInput{Snapshot: snapshot}, nil
}

type taskPerformanceCalculator struct {
	scoring *factorscoring.Executor
}

type taskPerformancePipelineResult struct {
	outcome *assessment.AssessmentOutcome
}

func (c taskPerformanceCalculator) Calculate(ctx context.Context, _ evalpipeline.CalculationInput) (any, error) {
	if c.scoring == nil {
		return nil, fmt.Errorf("task_performance evaluation calculator is not configured")
	}
	execInput, ok := evaluationexecute.ExecutionInputFromContext(ctx)
	if !ok {
		return nil, evaluationexecute.ErrDescriptorPipelineContext
	}
	scaleSnapshot, ok := portevaluationinput.CognitiveScaleSnapshot(execInput.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("cognitive model payload is required")
	}
	outcome, err := c.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: execInput.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(execInput.Input, scaleSnapshot),
	})
	if err != nil {
		return nil, err
	}
	return taskPerformancePipelineResult{outcome: outcome}, nil
}

type taskPerformanceOutcomeAssembler struct{}

func (taskPerformanceOutcomeAssembler) Assemble(result any) (any, error) {
	pipelineResult, ok := result.(taskPerformancePipelineResult)
	if !ok || pipelineResult.outcome == nil {
		return nil, fmt.Errorf("task_performance outcome assembler received invalid type %T", result)
	}
	return NormalizeOutcome(pipelineResult.outcome), nil
}
