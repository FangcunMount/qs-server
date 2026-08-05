package ruleengine

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

type scoringStrategy interface {
	Calculate(values []float64, params map[string]string) (float64, error)
	StrategyType() calculation.StrategyType
}

type scoringStrategies map[calculation.StrategyType]scoringStrategy

func newQuestionAggregationStrategies() scoringStrategies {
	// Public ScaleFactorScorer registry matches capability question_aggregation:
	// sum / avg(average) / cnt(count).
	strategies := scoringStrategies{}
	strategies.Register(&sumStrategy{})
	strategies.Register(&averageStrategy{})
	strategies.Register(&countStrategy{})
	return strategies
}

func (s scoringStrategies) Register(strategy scoringStrategy) {
	s[strategy.StrategyType()] = strategy
}

func (s scoringStrategies) Get(strategyType calculation.StrategyType) scoringStrategy {
	return s[strategyType]
}

type sumStrategy struct{}

func (s *sumStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	return sumValues(values), nil
}

func (s *sumStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeSum
}

type averageStrategy struct{}

func (s *averageStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	return sumValues(values) / float64(len(values)), nil
}

func (s *averageStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeAverage
}

type countStrategy struct{}

func (s *countStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	return float64(len(values)), nil
}

func (s *countStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeCount
}
