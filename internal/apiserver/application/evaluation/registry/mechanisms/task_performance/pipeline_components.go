package task_performance

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/inputinvariant"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
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

func (taskPerformanceInputAssembler) Assemble(input evalpipeline.ExecutionInput) (evalpipeline.CalculationInput, error) {
	route, ok := evaloutcome.ModelRouteFromInput(input.Input)
	if !ok {
		return evalpipeline.CalculationInput{}, fmt.Errorf("descriptor pipeline requires model route")
	}
	return evalpipeline.CalculationInput{Route: route, Execution: input}, nil
}

type taskPerformanceCalculator struct {
	scoring *factorscoring.Executor
}

type taskPerformancePipelineResult struct {
	outcome *domainoutcome.Execution
}

func (c taskPerformanceCalculator) Calculate(ctx context.Context, calcInput evalpipeline.CalculationInput) (any, error) {
	if c.scoring == nil {
		return nil, fmt.Errorf("task_performance evaluation calculator is not configured")
	}
	execInput := calcInput.Execution
	if err := inputinvariant.Validate(inputinvariant.Input{
		Assessment:    execInput.Assessment,
		Snapshot:      execInput.Input,
		DescriptorKey: "task_performance",
	}); err != nil {
		return nil, err
	}
	cognitiveSnapshot, ok := portevaluationinput.CognitiveExecutionSnapshot(execInput.Input)
	if !ok || cognitiveSnapshot == nil {
		return nil, fmt.Errorf("cognitive model payload is required")
	}
	abilityRules := portevaluationinput.AbilityConclusionsFromSnapshot(execInput.Input)
	if cognitiveSnapshot.SPM != nil {
		outcome, err := CalculateSPM(execInput.Input, cognitiveSnapshot)
		if err != nil {
			return nil, err
		}
		return taskPerformancePipelineResult{outcome: ApplyAbilityConclusions(outcome, abilityRules)}, nil
	}
	scaleSnapshot := cognitiveSnapshot.ToScaleSnapshot()
	outcome, err := c.scoring.ExecuteForDescriptor(ctx, evalpipeline.ExecutionInput{
		Assessment: execInput.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(execInput.Input, scaleSnapshot),
	}, "task_performance")
	if err != nil {
		return nil, err
	}
	return taskPerformancePipelineResult{outcome: ApplyAbilityConclusions(NormalizeOutcome(outcome), abilityRules)}, nil
}

type taskPerformanceOutcomeAssembler struct{}

func (taskPerformanceOutcomeAssembler) Assemble(result any) (any, error) {
	pipelineResult, ok := result.(taskPerformancePipelineResult)
	if !ok || pipelineResult.outcome == nil {
		return nil, fmt.Errorf("task_performance outcome assembler received invalid type %T", result)
	}
	return pipelineResult.outcome, nil
}
