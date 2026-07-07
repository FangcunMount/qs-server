package scoring

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainfactor_scoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 运行因子计分 评估s。
type Executor struct {
	validator InputValidator
	handler   *domainfactor_scoring.Handler
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建因子计分 评估 executor。
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithDeps(
		DefaultInputValidator{},
		domainfactor_scoring.NewHandler(scoringRegistry{scorer: scorer}),
	)
}

// NewExecutorWithDeps 创建executor 使用 replaceable dependencies (tests)。
func NewExecutorWithDeps(validator InputValidator, handler *domainfactor_scoring.Handler) *Executor {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if handler == nil {
		handler = domainfactor_scoring.NewDefaultHandler()
	}
	return &Executor{
		validator: validator,
		handler:   handler,
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

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.handler == nil {
		return nil, fmt.Errorf("factor_scoring evaluation executor is not configured")
	}
	executionInput := ExecutionInput{
		Assessment: input.Assessment,
		Input:      input.Input,
	}
	if err := e.validator.Validate(executionInput); err != nil {
		return nil, err
	}
	result, err := e.handler.Score(ctx, evaluateInputFromSnapshot(input.Input))
	if err != nil {
		return nil, err
	}
	return ToAssessmentOutcome(result, input.Assessment, input.Input), nil
}

type scoringRegistry struct {
	scorer ruleengine.ScaleFactorScorer
}

func (r scoringRegistry) ScoreFactor(ctx context.Context, factor scalesnapshot.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return domainfactor_scoring.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, factor.Code, values, factor.ScoringStrategy, nil)
}
