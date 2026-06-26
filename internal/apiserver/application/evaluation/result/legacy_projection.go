package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// legacyResultForPersistence projects the canonical outcome into the legacy write model.
// This is the single application-layer boundary for ToEvaluationResult until characterization migrates off LegacyResult().
func legacyResultForPersistence(outcome Outcome) *assessment.EvaluationResult {
	if outcome.Execution == nil {
		return nil
	}
	return outcome.Execution.ToEvaluationResult()
}
