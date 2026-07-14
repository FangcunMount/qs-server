package ruleengine

import "context"

// ScorableValue is the value surface needed by answer scoring execution.
type ScorableValue interface {
	IsEmpty() bool
	AsSingleSelection() (string, bool)
	AsMultipleSelections() ([]string, bool)
	AsNumber() (float64, bool)
}

// AnswerScoreTask describes one answer score execution request.
type AnswerScoreTask struct {
	ID           string
	Value        ScorableValue
	OptionScores map[string]float64
}

// AnswerScoreResult describes one answer score execution result.
type AnswerScoreResult struct {
	ID       string
	Score    float64
	MaxScore float64
}

// AnswerScorer executes answer scoring rules.
type AnswerScorer interface {
	ScoreAnswers(ctx context.Context, tasks []AnswerScoreTask) ([]AnswerScoreResult, error)
}

// ScaleFactorScorer is reserved for scale factor scoring engine adapters.
type ScaleFactorScorer interface {
	ScoreFactor(ctx context.Context, factorCode string, values []float64, strategy string, params map[string]string) (float64, error)
}
