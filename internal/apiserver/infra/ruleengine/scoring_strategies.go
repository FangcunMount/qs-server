package ruleengine

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

type scoringStrategy interface {
	Calculate(values []float64, params map[string]string) (float64, error)
	StrategyType() calculation.StrategyType
}

type scoringStrategies map[calculation.StrategyType]scoringStrategy

func newDefaultScoringStrategies() scoringStrategies {
	strategies := scoringStrategies{}
	strategies.Register(&sumStrategy{})
	strategies.Register(&averageStrategy{})
	strategies.Register(&weightedSumStrategy{})
	strategies.Register(&maxStrategy{})
	strategies.Register(&minStrategy{})
	strategies.Register(&countStrategy{})
	strategies.Register(&firstStrategy{})
	strategies.Register(&lastStrategy{})
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

type weightedSumStrategy struct{}

func (s *weightedSumStrategy) Calculate(values []float64, params map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	weights, err := parseWeights(params, len(values))
	if err != nil {
		return 0, err
	}
	var total float64
	for i, value := range values {
		total += value * weights[i]
	}
	return total, nil
}

func (s *weightedSumStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeWeightedSum
}

func parseWeights(params map[string]string, count int) ([]float64, error) {
	weightsText, ok := params[calculation.ParamKeyWeights]
	if !ok || weightsText == "" {
		weights := make([]float64, count)
		for i := range weights {
			weights[i] = 1
		}
		return weights, nil
	}

	var weights []float64
	if err := json.Unmarshal([]byte(weightsText), &weights); err != nil {
		return nil, fmt.Errorf("invalid weights format: %w", err)
	}
	if len(weights) != count {
		return nil, fmt.Errorf("weights count (%d) does not match values count (%d)", len(weights), count)
	}
	return weights, nil
}

type maxStrategy struct{}

func (s *maxStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	maxValue := math.Inf(-1)
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue, nil
}

func (s *maxStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeMax
}

type minStrategy struct{}

func (s *minStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	minValue := math.Inf(1)
	for _, value := range values {
		if value < minValue {
			minValue = value
		}
	}
	return minValue, nil
}

func (s *minStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeMin
}

type countStrategy struct{}

func (s *countStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	return float64(len(values)), nil
}

func (s *countStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeCount
}

type firstStrategy struct{}

func (s *firstStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	return values[0], nil
}

func (s *firstStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeFirst
}

type lastStrategy struct{}

func (s *lastStrategy) Calculate(values []float64, _ map[string]string) (float64, error) {
	if len(values) == 0 {
		return 0, nil
	}
	return values[len(values)-1], nil
}

func (s *lastStrategy) StrategyType() calculation.StrategyType {
	return calculation.StrategyTypeLast
}
