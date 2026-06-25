package scale

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scaleinterpretation"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 执行 Scale 解释模型评估
type Executor struct {
	service Service
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建 Scale 解释模型评估执行器
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithService(
		NewService(
			DefaultInputValidator{},
			evaluationdomain.NewScaleHandler(scaleScoringRegistry{scorer: scorer}),
			DefaultResultMapper{},
		),
	)
}

// NewExecutorWithService 创建 Scale 解释模型评估执行器
func NewExecutorWithService(service Service) *Executor {
	if service == nil {
		service = NewService(
			DefaultInputValidator{},
			evaluationdomain.NewDefaultScaleHandler(),
			DefaultResultMapper{},
		)
	}
	return &Executor{service: service}
}

func (e *Executor) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.EvaluationResult, error) {
	if e == nil || e.service == nil {
		return nil, fmt.Errorf("scale evaluation service is not configured")
	}
	return e.service.Evaluate(ctx, input.Assessment, input.Input)
}

type scaleScoringRegistry struct {
	scorer ruleengine.ScaleFactorScorer
}

func (r scaleScoringRegistry) ScoreFactor(ctx context.Context, factor rulesetscale.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return scaleinterpretation.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, factor.Code, values, factor.ScoringStrategy, nil)
}
