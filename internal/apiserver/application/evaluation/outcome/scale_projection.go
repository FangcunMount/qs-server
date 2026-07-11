package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// ScaleScoreProjectionFromExecution builds the Evaluation-owned scale query
// projection directly from the canonical Execution fact.
func ScaleScoreProjectionFromExecution(assessmentID assessment.ID, execution *domainoutcome.Execution) *assessment.ScaleScoreProjection {
	if execution == nil || !execution.ModelRef.IsScale() {
		return nil
	}
	var totalScore float64
	if execution.Primary != nil {
		totalScore = execution.Primary.Value
	}
	var riskLevel assessment.RiskLevel
	if execution.Level != nil && assessment.IsRiskLevelCode(execution.Level.Code) {
		riskLevel = assessment.RiskLevel(execution.Level.Code)
	}
	factorScores := scaleFactorScores(execution)
	return assessment.NewScaleScoreProjection(assessmentID, totalScore, riskLevel, factorScores)
}

func scaleFactorScores(execution *domainoutcome.Execution) []assessment.ScaleFactorScore {
	if details, ok := execution.Detail.Payload.([]assessment.FactorScoreResult); ok && len(details) > 0 {
		result := make([]assessment.ScaleFactorScore, 0, len(details))
		for _, detail := range details {
			result = append(result, assessment.NewScaleFactorScore(
				detail.FactorCode, detail.FactorName, detail.RawScore, detail.RiskLevel, detail.IsTotalScore,
			))
		}
		return result
	}
	result := make([]assessment.ScaleFactorScore, 0, len(execution.Dimensions))
	for _, dimension := range execution.Dimensions {
		if dimension.Score == nil {
			continue
		}
		riskLevel := assessment.RiskLevelNone
		if dimension.Level != nil && assessment.IsRiskLevelCode(dimension.Level.Code) {
			riskLevel = assessment.RiskLevel(dimension.Level.Code)
		}
		result = append(result, assessment.NewScaleFactorScore(
			assessment.NewFactorCode(dimension.Code), dimension.Name, dimension.Score.Value, riskLevel, dimension.Role == "total",
		))
	}
	return result
}
