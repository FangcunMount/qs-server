package scoring

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 运行因子计分 评估s。
type Executor struct {
	validator InputValidator
	evaluator *calcscoring.Evaluator
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建因子计分 评估 executor。
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithDeps(
		DefaultInputValidator{},
		calcscoring.NewEvaluator(scoringRegistry{scorer: scorer}),
	)
}

// NewExecutorWithDeps 创建executor 使用 replaceable dependencies (tests)。
func NewExecutorWithDeps(validator InputValidator, evaluator *calcscoring.Evaluator) *Executor {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if evaluator == nil {
		evaluator = calcscoring.NewDefaultEvaluator()
	}
	return &Executor{
		validator: validator,
		evaluator: evaluator,
	}
}

func (e *Executor) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (e *Executor) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}

func (e *Executor) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathScaleDescriptor
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*domainoutcome.Execution, error) {
	if e == nil || e.evaluator == nil {
		return nil, fmt.Errorf("factor_scoring evaluation executor is not configured")
	}
	executionInput := ExecutionInput{
		Assessment: input.Assessment,
		Input:      input.Input,
	}
	if err := e.validator.Validate(executionInput); err != nil {
		return nil, err
	}
	result, err := e.evaluator.Score(ctx, calcInputFromSnapshot(input.Input))
	if err != nil {
		return nil, err
	}
	return ToExecution(result, input.Assessment, input.Input), nil
}

type scoringRegistry struct {
	scorer ruleengine.ScaleFactorScorer
}

func (r scoringRegistry) ScoreFactor(ctx context.Context, factor calcscoring.Factor, values []float64) (float64, error) {
	if r.scorer == nil {
		return calcscoring.DefaultStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, factor.Code, values, factor.ScoringStrategy, nil)
}
