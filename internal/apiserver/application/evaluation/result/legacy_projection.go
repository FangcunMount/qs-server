package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// legacyResultForPersistence projects AssessmentOutcome into EvaluationResult for persistence adapters only.
// Application write paths must use AssessmentOutcome directly; characterization reads Execution.
func legacyResultForPersistence(outcome Outcome) *assessment.EvaluationResult {
	if outcome.Execution == nil {
		return nil
	}
	return outcome.Execution.ToEvaluationResult()
}

func outcomeFromLegacyEvaluationResult(result *assessment.EvaluationResult) *assessment.AssessmentOutcome {
	return assessment.AssessmentOutcomeFromEvaluationResult(result) //nolint:staticcheck // single boundary adapter for characterization
}
