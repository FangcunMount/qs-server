package scale

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 执行 Scale 解释模型评估
type Executor struct {
	service Service
}

// Executor 实现 evaluationexecute.Evaluator 接口
// 场景：评估引擎执行 Scale 解释模型评估
// 流程：
//  1. 实现 evaluationexecute.Evaluator 接口
//  2. 返回评估器
var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建 Scale 解释模型评估执行器
// 使用默认评分策略注册表
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutorWithService(
		NewService(
			DefaultInputValidator{},
			DefaultInputAssembler{},
			scaleinterpretation.NewEvaluator(scaleScoringRegistry{scorer: scorer}),
			DefaultResultMapper{},
		),
	)
}

// NewExecutorWithService 创建 Scale 解释模型评估执行器
func NewExecutorWithService(service Service) *Executor {
	if service == nil {
		service = NewService(
			DefaultInputValidator{},
			DefaultInputAssembler{},
			scaleinterpretation.NewDefaultEvaluator(),
			DefaultResultMapper{},
		)
	}
	return &Executor{service: service}
}

// Kind 返回评估模型类型
func (e *Executor) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

// Execute 执行 Scale 解释模型评估
// 场景：评估引擎执行 Scale 解释模型评估
// 流程：
//  1. 验证输入是否合法
//  2. 执行 Scale 解释模型评估
//  3. 返回评估结果
func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.EvaluationResult, error) {
	if e == nil || e.service == nil {
		return nil, fmt.Errorf("scale evaluation service is not configured")
	}
	return e.service.Evaluate(ctx, input.Assessment, input.Input)
}

// scaleScoringRegistry 评分策略注册表
type scaleScoringRegistry struct {
	// 评分器
	scorer ruleengine.ScaleFactorScorer
}

// ScoreFactor 评分因子
// 场景：评估引擎执行 Scale 解释模型评估
// 流程：
//  1. 获取评分器
//  2. 评分因子
//  3. 返回评分结果
func (r scaleScoringRegistry) ScoreFactor(ctx context.Context, factor domainScale.FactorSnapshot, values []float64) (float64, error) {
	if r.scorer == nil {
		return scaleinterpretation.DefaultScoringStrategyRegistry{}.ScoreFactor(ctx, factor, values)
	}
	return r.scorer.ScoreFactor(ctx, string(factor.Code), values, string(factor.ScoringStrategy), nil)
}
