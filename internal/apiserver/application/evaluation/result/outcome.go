package result

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// EvaluationOutcome is the canonical execution result before legacy projections.
type EvaluationOutcome struct {
	ModelRef assessment.EvaluationModelRef
	Summary  assessment.ResultSummary
	Detail   assessment.EvaluationDetail
}

func NewEvaluationOutcome(
	modelRef assessment.EvaluationModelRef,
	summary assessment.ResultSummary,
	detail assessment.EvaluationDetail,
) EvaluationOutcome {
	if detail.Kind == "" {
		detail.Kind = modelRef.Kind()
	}
	return EvaluationOutcome{
		ModelRef: modelRef,
		Summary:  summary,
		Detail:   detail,
	}
}

func EvaluationOutcomeFromResult(result *assessment.EvaluationResult) EvaluationOutcome {
	if result == nil {
		return EvaluationOutcome{}
	}
	return EvaluationOutcome{
		ModelRef: result.ModelRef,
		Summary:  result.Summary,
		Detail:   result.Detail,
	}
}

func (o EvaluationOutcome) ToEvaluationResult() *assessment.EvaluationResult {
	result := assessment.NewModelEvaluationResult(o.ModelRef, o.Summary, o.Detail)
	if o.Detail.Kind == assessment.EvaluationModelKindScale || o.ModelRef.IsScale() {
		return applyScaleProjection(result, o)
	}
	return result
}

func applyScaleProjection(result *assessment.EvaluationResult, o EvaluationOutcome) *assessment.EvaluationResult {
	if result == nil {
		return nil
	}
	if scores, ok := o.Detail.Payload.([]assessment.FactorScoreResult); ok && len(scores) > 0 {
		result.FactorScores = scores
	}
	if o.Summary.Score != nil {
		result.TotalScore = *o.Summary.Score
	}
	if o.Summary.Level != nil {
		result.RiskLevel = assessment.RiskLevel(*o.Summary.Level)
	}
	if o.Summary.PrimaryLabel != "" {
		result.Conclusion = o.Summary.PrimaryLabel
	}
	return result
}
