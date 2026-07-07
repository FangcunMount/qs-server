// Package scale is a transitional re-export of application/evaluation/factor_scoring.
// New code should import factor_scoring directly.
package scale

import (
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type (
	Executor              = factorscoring.Executor
	InputValidator        = factorscoring.InputValidator
	DefaultInputValidator = factorscoring.DefaultInputValidator
	ScaleExecutionInput   = factorscoring.ExecutionInput
)

var (
	NewExecutor         = factorscoring.NewExecutor
	NewExecutorWithDeps = factorscoring.NewExecutorWithDeps
	ToAssessmentOutcome = factorscoring.ToAssessmentOutcome
)

// Deprecated: use factor_scoring.NewExecutor.
func NewScaleExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutor(scorer)
}
