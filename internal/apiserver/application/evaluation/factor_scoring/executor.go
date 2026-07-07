package factor_scoring

import (
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor runs factor-scoring evaluations via the shared scale scoring engine.
type Executor = scaleEvaluation.Executor

// NewExecutor creates a factor-scoring evaluation executor.
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return scaleEvaluation.NewExecutor(scorer)
}
