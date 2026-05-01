package ruleengine

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// AnswerScorer adapts the current calculation batch engine to the application port.
type AnswerScorer struct {
	batch *calculation.BatchScorer
}

// NewAnswerScorer creates an answer scoring engine adapter.
func NewAnswerScorer(batch *calculation.BatchScorer) *AnswerScorer {
	if batch == nil {
		batch = calculation.NewBatchScorer()
	}
	return &AnswerScorer{batch: batch}
}

// ScoreAnswers executes answer scoring with the current calculation engine.
func (s *AnswerScorer) ScoreAnswers(_ context.Context, tasks []ruleengineport.AnswerScoreTask) ([]ruleengineport.AnswerScoreResult, error) {
	scoreTasks := make([]calculation.ScoreTask, 0, len(tasks))
	for _, task := range tasks {
		scoreTasks = append(scoreTasks, calculation.ScoreTask{
			ID:           task.ID,
			Value:        task.Value,
			OptionScores: task.OptionScores,
		})
	}

	results := s.batch.ScoreAll(scoreTasks)
	output := make([]ruleengineport.AnswerScoreResult, 0, len(results))
	for _, result := range results {
		output = append(output, ruleengineport.AnswerScoreResult{
			ID:       result.ID,
			Score:    result.Score,
			MaxScore: result.MaxScore,
		})
	}
	return output, nil
}

// ScaleFactorScorer adapts the current calculation strategies to the factor-scoring port.
type ScaleFactorScorer struct{}

// NewScaleFactorScorer creates a factor scoring engine adapter.
func NewScaleFactorScorer() *ScaleFactorScorer {
	return &ScaleFactorScorer{}
}

// ScoreFactor executes a factor aggregation strategy over prepared numeric values.
func (s *ScaleFactorScorer) ScoreFactor(_ context.Context, factorCode string, values []float64, strategy string, params map[string]string) (float64, error) {
	switch strategy {
	case "sum":
		return calculateWithFallback(calculation.StrategyTypeSum, values, params, sumValues), nil
	case "avg", "average":
		if len(values) == 0 {
			return 0, nil
		}
		return calculateWithFallback(calculation.StrategyTypeAverage, values, params, func(values []float64) float64 {
			return sumValues(values) / float64(len(values))
		}), nil
	case "cnt", "count":
		return calculateWithFallback(calculation.StrategyTypeCount, values, params, func(values []float64) float64 {
			return float64(len(values))
		}), nil
	default:
		return 0, fmt.Errorf("unknown factor scoring strategy for %s: %s", factorCode, strategy)
	}
}

func calculateWithFallback(strategyType calculation.StrategyType, values []float64, params map[string]string, fallback func([]float64) float64) float64 {
	if strategy := calculation.GetStrategy(strategyType); strategy != nil {
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
