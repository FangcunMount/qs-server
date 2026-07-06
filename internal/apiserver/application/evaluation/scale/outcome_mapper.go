package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ToAssessmentOutcome maps a scale interpretation result into the canonical domain outcome.
func ToAssessmentOutcome(
	result *evaluationscale.ScaleInterpretationResult,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *assessment.AssessmentOutcome {
	if result == nil {
		return nil
	}
	factorScores := factorScoreResultsFromInterpretation(result)
	level := string(result.RiskLevel)
	summaryScore := result.TotalScore
	outcome := assessment.NewAssessmentOutcome(
		scaleModelRef(a, snapshot),
		assessment.ResultSummary{
			PrimaryLabel: level,
			Score:        &summaryScore,
			Level:        &level,
		},
		assessment.EvaluationDetail{
			Kind:    assessment.EvaluationModelKindScale,
			Payload: factorScores,
		},
	)
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindRawTotal,
		Value: result.TotalScore,
	}
	if result.RiskLevel != "" {
		outcome.Level = &assessment.OutcomeResultLevel{
			Code:  string(result.RiskLevel),
			Label: string(result.RiskLevel),
		}
	}
	outcome.Dimensions = assessmentDimensionResultsFromFactorScores(factorScores)
	return outcome
}

func scaleModelRef(a *assessment.Assessment, snapshot *evaluationinput.InputSnapshot) assessment.EvaluationModelRef {
	if a != nil && a.EvaluationModelRef() != nil {
		return *a.EvaluationModelRef()
	}
	if snapshot != nil && snapshot.Model != nil {
		return assessment.NewEvaluationModelRefByCode(
			assessment.EvaluationModelKind(snapshot.Model.Kind),
			meta.NewCode(snapshot.Model.Code),
			snapshot.Model.Version,
			snapshot.Model.Title,
		)
	}
	return assessment.EvaluationModelRef{}
}

func factorScoreResultsFromInterpretation(result *evaluationscale.ScaleInterpretationResult) []assessment.FactorScoreResult {
	factorScores := make([]assessment.FactorScoreResult, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, assessment.NewFactorScoreResult(
			assessment.NewFactorCode(fs.FactorCode),
			fs.FactorName,
			fs.RawScore,
			assessment.RiskLevel(fs.RiskLevel),
			"",
			"",
			fs.IsTotalScore,
		))
	}
	return factorScores
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
