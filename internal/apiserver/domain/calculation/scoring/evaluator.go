package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// Evaluator executes scale factor scoring and risk classification.
type Evaluator struct {
	scoringRegistry StrategyRegistry
	calculator      *calculation.Engine
}

// StrategyRegistry executes scale factor aggregation strategies.
type StrategyRegistry interface {
	ScoreFactor(ctx context.Context, factor Factor, values []float64) (float64, error)
}

// NewEvaluator creates a scale scoring evaluator.
func NewEvaluator(scoringRegistry StrategyRegistry) *Evaluator {
	if scoringRegistry == nil {
		scoringRegistry = DefaultStrategyRegistry{}
	}
	return &Evaluator{
		scoringRegistry: scoringRegistry,
		calculator:      calculation.NewEngine(scaleCalculationRegistry{registry: scoringRegistry}),
	}
}

// NewDefaultEvaluator creates the default scale scoring evaluator.
func NewDefaultEvaluator() *Evaluator {
	return NewEvaluator(DefaultStrategyRegistry{})
}

// Score executes scale scoring and risk classification without interpretation copy.
func (e *Evaluator) Score(ctx context.Context, input Input) (*Result, error) {
	factorScores, totalScore, riskLevel, err := e.runScoring(ctx, input)
	if err != nil {
		return nil, err
	}
	return &Result{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		FactorScores: factorScores,
	}, nil
}

// DefaultStrategyRegistry is the default scale factor aggregation registry.
type DefaultStrategyRegistry struct{}

// ScoreFactor executes scale factor aggregation strategies.
func (DefaultStrategyRegistry) ScoreFactor(_ context.Context, factor Factor, values []float64) (float64, error) {
	score, err := calculation.DefaultStrategyRegistry{}.Score(context.Background(), calculation.Dimension{
		Code:         factor.Code,
		StrategyCode: factor.ScoringStrategy,
	}, values)
	if err != nil {
		return 0, err
	}
	strategy := Strategy(factor.ScoringStrategy)
	if strategy != StrategySum &&
		strategy != StrategyAvg &&
		strategy != StrategyCnt {
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
	return score, nil
}

type scaleCalculationRegistry struct {
	registry StrategyRegistry
}

func (r scaleCalculationRegistry) Score(ctx context.Context, dimension calculation.Dimension, values []float64) (float64, error) {
	if r.registry == nil {
		return 0, nil
	}
	return r.registry.ScoreFactor(ctx, Factor{
		Code:            dimension.Code,
		ScoringStrategy: dimension.StrategyCode,
	}, values)
}
