package task_performance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// NormalizeOutcome reuses the shared calculationadapter outcome bridge for cognitive/task_performance runs.
func NormalizeOutcome(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome {
	if outcome == nil {
		return nil
	}
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calculationadapter.CalcResultFromOutcome(outcome))
}
