// Package behavioralrating is a transitional re-export of application/evaluation/factor_norm.
package behavioralrating

import (
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type Executor = factornorm.Executor

var (
	NewExecutor            = factornorm.NewExecutor
	ApplyFactorProjections = factornorm.ApplyFactorProjections
	ApplyNormProjection    = factornorm.ApplyNormProjection
	NormSubjectFromInput   = factornorm.NormSubjectFromInput
)

// Deprecated: use factor_norm.NewExecutor.
func NewBehavioralRatingExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return NewExecutor(scorer)
}
