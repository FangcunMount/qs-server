package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// legacyResultForPersistence projects the canonical outcome into the legacy write model.
// Keep this as the single persistence boundary until ApplyEvaluation accepts AssessmentOutcome.
func legacyResultForPersistence(outcome Outcome) *assessment.EvaluationResult {
	if outcome.Execution == nil {
		return nil
	}
	return outcome.Execution.ToEvaluationResult()
}
