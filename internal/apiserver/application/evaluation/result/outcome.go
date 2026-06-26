package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// EvaluationOutcome is the application-layer alias of the canonical domain outcome.
type EvaluationOutcome = assessment.AssessmentOutcome

func NewEvaluationOutcome(
	modelRef assessment.EvaluationModelRef,
	summary assessment.ResultSummary,
	detail assessment.EvaluationDetail,
) EvaluationOutcome {
	return *assessment.NewAssessmentOutcome(modelRef, summary, detail)
}

func EvaluationOutcomeFromResult(result *assessment.EvaluationResult) EvaluationOutcome {
	outcome := assessment.AssessmentOutcomeFromEvaluationResult(result)
	if outcome == nil {
		return EvaluationOutcome{}
	}
	return *outcome
}

func EvaluationOutcomeToEvaluationResult(o EvaluationOutcome) *assessment.EvaluationResult {
	return o.ToEvaluationResult()
}
