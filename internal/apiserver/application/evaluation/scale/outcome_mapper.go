package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ToAssessmentOutcome maps a scale interpretation result into the canonical domain outcome.
func ToAssessmentOutcome(
	result *evaluationscale.ScaleInterpretationResult,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *assessment.AssessmentOutcome {
	legacy := DefaultResultMapper{}.ToEvaluationResult(result, a, snapshot)
	if legacy == nil {
		return nil
	}
	outcome := assessment.AssessmentOutcomeFromEvaluationResult(legacy)
	if outcome == nil {
		return nil
	}
	outcome.Dimensions = assessmentDimensionResultsFromFactorScores(legacy.FactorScores)
	return outcome
}

func assessmentDimensionResultsFromFactorScores(scores []assessment.FactorScoreResult) []assessment.DimensionResult {
	results := make([]assessment.DimensionResult, 0, len(scores))
	for _, score := range scores {
		dim := assessment.DimensionResult{
			Code: score.FactorCode.String(),
			Name: score.FactorName,
			Kind: assessment.DimensionKindFactor,
			Score: &assessment.OutcomeScoreValue{
				Kind:  assessment.OutcomeScoreKindRawTotal,
				Value: score.RawScore,
			},
			Description: score.Conclusion,
			Suggestion:  score.Suggestion,
		}
		if score.RiskLevel != "" {
			dim.Level = &assessment.OutcomeResultLevel{Code: string(score.RiskLevel)}
		}
		results = append(results, dim)
	}
	return results
}
