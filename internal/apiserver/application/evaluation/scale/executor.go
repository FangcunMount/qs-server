package scale

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 执行 Scale 解释模型评估。
type Executor struct {
	validator InputValidator
	handler   *evaluationscale.Handler
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建 Scale 解释模型评估执行器。
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithDeps(
		DefaultInputValidator{},
		evaluationscale.NewHandler(scaleScoringRegistry{scorer: scorer}),
	)
}

// NewExecutorWithDeps 创建带可替换依赖的 Scale 执行器（测试用）。
func NewExecutorWithDeps(validator InputValidator, handler *evaluationscale.Handler) *Executor {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if handler == nil {
		handler = evaluationscale.NewDefaultHandler()
	}
	return &Executor{
		validator: validator,
		handler:   handler,
	}
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyScaleDefault
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.handler == nil {
		return nil, fmt.Errorf("scale evaluation executor is not configured")
	}
	executionInput := ScaleExecutionInput{
		Assessment: input.Assessment,
		Input:      input.Input,
	}
	if err := e.validator.Validate(executionInput); err != nil {
		return nil, err
	}
	result, err := e.handler.Evaluate(ctx, scaleEvaluateInputFromSnapshot(input.Input))
	if err != nil {
		return nil, err
	}
	return ToAssessmentOutcome(result, input.Assessment, input.Input), nil
}

type scaleScoringRegistry struct {
	scorer ruleengine.ScaleFactorScorer
}

func (r scaleScoringRegistry) ScoreFactor(ctx context.Context, factor scalesnapshot.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return evaluationscale.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, factor.Code, values, factor.ScoringStrategy, nil)
}
