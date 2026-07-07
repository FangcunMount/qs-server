package task_performance

import (
	cognitiveEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor runs task-performance evaluations via the shared scale scoring engine.
type Executor = cognitiveEvaluation.Executor

// NewExecutor creates a task-performance evaluation executor.
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return cognitiveEvaluation.NewExecutor(scorer)
}
