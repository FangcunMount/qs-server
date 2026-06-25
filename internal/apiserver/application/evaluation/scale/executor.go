package scale

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
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

func (r scaleScoringRegistry) ScoreFactor(ctx context.Context, factor domainScale.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return scaleinterpretation.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, string(factor.Code), values, string(factor.ScoringStrategy), nil)
}
