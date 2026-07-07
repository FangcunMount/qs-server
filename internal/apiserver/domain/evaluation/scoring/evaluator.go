package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

// Evaluator 执行量表解释模型评估。
type Evaluator struct {
	scoringRegistry ScoringStrategyRegistry
	calculator      *calculation.Engine
}

// NewEvaluator 创建量表解释模型评估器。
func NewEvaluator(scoringRegistry ScoringStrategyRegistry) *Evaluator {
	if scoringRegistry == nil {
		scoringRegistry = DefaultScoringStrategyRegistry{}
	}
	return &Evaluator{
		scoringRegistry: scoringRegistry,
		calculator:      calculation.NewEngine(scaleCalculationRegistry{registry: scoringRegistry}),
	}
}

// NewDefaultEvaluator 创建默认量表解释模型评估器。
func NewDefaultEvaluator() *Evaluator {
	return NewEvaluator(DefaultScoringStrategyRegistry{})
}

// Score 执行量表计分与风险分级，不生成解读文案（文案由解读侧依据模型规则生成）。
func (e *Evaluator) Score(ctx context.Context, input ScaleInterpretationInput) (*ScaleInterpretationResult, error) {
	factorScores, totalScore, riskLevel, err := e.runScoring(ctx, input)
	if err != nil {
		return nil, err
	}
	return &ScaleInterpretationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		FactorScores: factorScores,
	}, nil
}

// DefaultScoringStrategyRegistry 默认量表因子聚合策略注册表。
type DefaultScoringStrategyRegistry struct{}

// ScoreFactor 执行量表因子聚合策略。
func (DefaultScoringStrategyRegistry) ScoreFactor(_ context.Context, factor scalesnapshot.FactorSnapshot, values []float64) (float64, error) {
	score, err := calculation.DefaultStrategyRegistry{}.Score(context.Background(), calculation.Dimension{
		Code:         factor.Code,
		StrategyCode: factor.ScoringStrategy,
	}, values)
	if err != nil {
		return 0, err
	}
	strategy := ScoringStrategy(factor.ScoringStrategy)
	if strategy != ScoringStrategySum &&
		strategy != ScoringStrategyAvg &&
		strategy != ScoringStrategyCnt {
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
	return score, nil
}

type scaleCalculationRegistry struct {
	registry ScoringStrategyRegistry
}

func (r scaleCalculationRegistry) Score(ctx context.Context, dimension calculation.Dimension, values []float64) (float64, error) {
	if r.registry == nil {
		return 0, nil
	}
	return r.registry.ScoreFactor(ctx, scalesnapshot.FactorSnapshot{
		Code:            dimension.Code,
		ScoringStrategy: dimension.StrategyCode,
	}, values)
}
