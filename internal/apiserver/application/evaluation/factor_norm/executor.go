package factor_norm

import (
	behavioralratingEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/behavioral_rating"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor runs factor-norm evaluations via the shared scale scoring engine.
type Executor = behavioralratingEvaluation.Executor

// NewExecutor creates a factor-norm evaluation executor.
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return behavioralratingEvaluation.NewExecutor(scorer)
}
