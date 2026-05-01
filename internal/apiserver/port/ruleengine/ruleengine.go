package ruleengine

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
)

// AnswerValidationTask describes one answer validation execution request.
type AnswerValidationTask struct {
	ID    string
	Value validation.ValidatableValue
	Rules []validation.ValidationRule
}

// ValidationError is a stable application-facing validation error.
type ValidationError struct {
	RuleType string
	Message  string
}

// AnswerValidationResult describes one answer validation execution result.
type AnswerValidationResult struct {
	ID     string
	Valid  bool
	Errors []ValidationError
}

// AnswerValidator executes answer validation rules.
type AnswerValidator interface {
	ValidateAnswers(ctx context.Context, tasks []AnswerValidationTask) ([]AnswerValidationResult, error)
}

// AnswerScoreTask describes one answer score execution request.
type AnswerScoreTask struct {
	ID           string
	Value        calculation.ScorableValue
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
