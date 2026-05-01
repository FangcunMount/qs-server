package ruleengine

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// AnswerScorer executes answer scoring through the infrastructure rule engine.
type AnswerScorer struct {
	scorer *optionScorer
}

// NewAnswerScorer creates an answer scoring engine adapter.
func NewAnswerScorer() *AnswerScorer {
	return &AnswerScorer{scorer: &optionScorer{}}
}

// ScoreAnswers executes answer scoring with the current rule engine.
func (s *AnswerScorer) ScoreAnswers(_ context.Context, tasks []ruleengineport.AnswerScoreTask) ([]ruleengineport.AnswerScoreResult, error) {
	scorer := s.scorer
	if scorer == nil {
		scorer = &optionScorer{}
	}
	output := make([]ruleengineport.AnswerScoreResult, 0, len(tasks))
	for _, task := range tasks {
		score, maxScore := scorer.scoreWithMax(task.Value, task.OptionScores)
		output = append(output, ruleengineport.AnswerScoreResult{
			ID:       task.ID,
			Score:    score,
			MaxScore: maxScore,
		})
	}
	return output, nil
}

type optionScorer struct{}

func (s *optionScorer) score(value ruleengineport.ScorableValue, optionScores map[string]float64) float64 {
	if value == nil || value.IsEmpty() || len(optionScores) == 0 {
		return 0
	}
	if selected, ok := value.AsSingleSelection(); ok {
		if score, found := optionScores[selected]; found {
			return score
		}
		return 0
	}
	if selections, ok := value.AsMultipleSelections(); ok {
		var total float64
		for _, selection := range selections {
			if score, found := optionScores[selection]; found {
				total += score
			}
		}
		return total
	}
	if number, ok := value.AsNumber(); ok {
		return number
	}
	return 0
}

func (s *optionScorer) scoreWithMax(value ruleengineport.ScorableValue, optionScores map[string]float64) (float64, float64) {
	return s.score(value, optionScores), maxOptionScore(optionScores)
}

func maxOptionScore(optionScores map[string]float64) float64 {
	var maxScore float64
	for _, score := range optionScores {
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// ScaleFactorScorer executes scale factor scoring through infrastructure strategies.
type ScaleFactorScorer struct {
	strategies scoringStrategies
}

// NewScaleFactorScorer creates a factor scoring engine adapter.
func NewScaleFactorScorer() *ScaleFactorScorer {
	return &ScaleFactorScorer{strategies: newDefaultScoringStrategies()}
}

// ScoreFactor executes a factor aggregation strategy over prepared numeric values.
func (s *ScaleFactorScorer) ScoreFactor(_ context.Context, factorCode string, values []float64, strategy string, params map[string]string) (float64, error) {
	switch strategy {
	case "sum":
		return s.calculateWithFallback(calculation.StrategyTypeSum, values, params, sumValues), nil
	case "avg", "average":
		if len(values) == 0 {
			return 0, nil
		}
		return s.calculateWithFallback(calculation.StrategyTypeAverage, values, params, func(values []float64) float64 {
			return sumValues(values) / float64(len(values))
		}), nil
	case "cnt", "count":
		return s.calculateWithFallback(calculation.StrategyTypeCount, values, params, func(values []float64) float64 {
			return float64(len(values))
		}), nil
	default:
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factorCode, strategy)
	}
}

func (s *ScaleFactorScorer) calculateWithFallback(strategyType calculation.StrategyType, values []float64, params map[string]string, fallback func([]float64) float64) float64 {
	if strategy := s.strategies.Get(strategyType); strategy != nil {
		if score, err := strategy.Calculate(values, params); err == nil {
			return score
		}
	}
	return fallback(values)
}

func sumValues(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}
