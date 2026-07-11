package calculationadapter

import (
	evaluationoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// ExecutionFromScoringInterpretation maps factor-scoring output to the
// canonical in-memory Evaluation result.
func ExecutionFromScoringInterpretation(
	result *scoring.Result,
	modelRef assessment.EvaluationModelRef,
) *domainoutcome.Execution {
	if result == nil {
		return nil
	}
	factorScores := factorScoreResultsFromInterpretation(result)
	level := string(result.RiskLevel)
	summaryScore := result.TotalScore
	legacy := assessment.NewAssessmentOutcome(
		modelRef,
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
	legacy.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindRawTotal,
		Value: result.TotalScore,
	}
	if result.RiskLevel != "" {
		legacy.Level = &assessment.OutcomeResultLevel{
			Code:  string(result.RiskLevel),
			Label: string(result.RiskLevel),
		}
	}
	legacy.Dimensions = assessmentDimensionResultsFromFactorScores(factorScores)
	return evaluationoutcome.ExecutionFromAssessmentOutcome(legacy)
}

// AssessmentOutcomeFromScoringInterpretation remains for compatibility with
// callers that have not yet moved to the Execution name.
//
// Deprecated: use ExecutionFromScoringInterpretation.
func AssessmentOutcomeFromScoringInterpretation(
	result *scoring.Result,
	modelRef assessment.EvaluationModelRef,
) *domainoutcome.Execution {
	return ExecutionFromScoringInterpretation(result, modelRef)
}

func factorScoreResultsFromInterpretation(result *scoring.Result) []assessment.FactorScoreResult {
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
