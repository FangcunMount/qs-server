package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
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

// ScoreFactor executes scale factor aggregation strategies against the capability catalog.
func (DefaultStrategyRegistry) ScoreFactor(_ context.Context, factor Factor, values []float64) (float64, error) {
	usage := usageForFactor(factor)
	code, ok := capability.Canonical(capability.PathScaleDescriptor, usage, factor.ScoringStrategy)
	if !ok {
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
	switch code {
	case "sum", "weighted_sum":
		// weighted_sum values are pre-multiplied by child weights in collectChildValues.
		return sumValues(values), nil
	case "avg":
		if len(values) == 0 {
			return 0, nil
		}
		return sumValues(values) / float64(len(values)), nil
	case "cnt":
		return float64(len(values)), nil
	case "none", "lookup", "custom":
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
}

func sumValues(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

type scaleCalculationRegistry struct {
	registry StrategyRegistry
}

func (r scaleCalculationRegistry) Score(ctx context.Context, dimension calculation.Dimension, values []float64) (float64, error) {
	if r.registry == nil {
		return 0, nil
	}
	factor := Factor{Code: dimension.Code, ScoringStrategy: dimension.StrategyCode}
	if capability.Supports(capability.PathScaleDescriptor, capability.UsageCompositeProjection, dimension.StrategyCode) &&
		!capability.Supports(capability.PathScaleDescriptor, capability.UsageQuestionAggregation, dimension.StrategyCode) {
		factor.ChildCodes = []string{"_"}
	}
	return r.registry.ScoreFactor(ctx, factor, values)
}
