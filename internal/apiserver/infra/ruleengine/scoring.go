package ruleengine

import (
	"context"

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
