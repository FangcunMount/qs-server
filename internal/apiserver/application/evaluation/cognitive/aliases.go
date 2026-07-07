// Package cognitive is a transitional re-export of application/evaluation/task_performance.
package cognitive

import (
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/task_performance"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type Executor = taskperformance.Executor

var NewExecutor = taskperformance.NewExecutor

// Deprecated: use task_performance.NewExecutor.
func NewCognitiveExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutor(scorer)
}
