package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func scoreRowToResult(row *evaluationreadmodel.ScoreRow, medicalScale *scalesnapshot.ScaleSnapshot) *ScoreResult {
	if row == nil {
		return nil
	}
	factorMaxScoreMap := factorMaxScores(medicalScale)
	factorScores := make([]FactorScoreResult, 0, len(row.FactorScores))
	for _, fs := range row.FactorScores {
		factorScores = append(factorScores, FactorScoreResult{
			FactorCode:   fs.FactorCode,
			FactorName:   fs.FactorName,
			RawScore:     fs.RawScore,
			MaxScore:     factorMaxScoreMap[fs.FactorCode],
			RiskLevel:    fs.RiskLevel,
			IsTotalScore: fs.IsTotalScore,
		})
	}
	return &ScoreResult{
		AssessmentID: row.AssessmentID,
		TotalScore:   row.TotalScore,
		RiskLevel:    row.RiskLevel,
		FactorScores: factorScores,
	}
}

func scoreResultFromScaleProjection(projection *assessment.ScaleScoreProjection, medicalScale *scalesnapshot.ScaleSnapshot) *ScoreResult {
	if projection == nil {
		return nil
	}
	factorMaxScoreMap := factorMaxScores(medicalScale)
	factorScores := make([]FactorScoreResult, 0, len(projection.FactorScores()))
	for _, score := range projection.FactorScores() {
		factorCode := score.FactorCode().String()
		factorScores = append(factorScores, FactorScoreResult{
			FactorCode:   factorCode,
			FactorName:   score.FactorName(),
			RawScore:     score.RawScore(),
			MaxScore:     factorMaxScoreMap[factorCode],
			RiskLevel:    string(score.RiskLevel()),
			IsTotalScore: score.IsTotalScore(),
		})
	}
	return &ScoreResult{
		AssessmentID: projection.AssessmentID().Uint64(),
		TotalScore:   projection.TotalScore(),
		RiskLevel:    string(projection.RiskLevel()),
		FactorScores: factorScores,
	}
}

func highRiskFactorsResultFromScoreRow(assessmentID uint64, row *evaluationreadmodel.ScoreRow, medicalScale *scalesnapshot.ScaleSnapshot) *HighRiskFactorsResult {
	if row == nil {
		return emptyHighRiskFactorsResult(assessmentID)
	}

	return highRiskFactorsResultFromScoreResult(scoreRowToResult(row, medicalScale))
}

func highRiskFactorsResultFromScoreResult(scoreResult *ScoreResult) *HighRiskFactorsResult {
	if scoreResult == nil {
		return emptyHighRiskFactorsResult(0)
	}
	highRiskFactors := make([]FactorScoreResult, 0)
	for _, fs := range scoreResult.FactorScores {
		if fs.RiskLevel == string(assessment.RiskLevelHigh) || fs.RiskLevel == string(assessment.RiskLevelSevere) {
			highRiskFactors = append(highRiskFactors, fs)
		}
	}
	needsUrgentCare := scoreResult.RiskLevel == string(assessment.RiskLevelSevere) || len(highRiskFactors) >= 3
	return &HighRiskFactorsResult{
		AssessmentID:    scoreResult.AssessmentID,
		HasHighRisk:     len(highRiskFactors) > 0,
		HighRiskFactors: highRiskFactors,
		NeedsUrgentCare: needsUrgentCare,
	}
}

func factorMaxScores(medicalScale *scalesnapshot.ScaleSnapshot) map[string]*float64 {
	result := make(map[string]*float64)
	if medicalScale == nil {
		return result
	}
	for _, f := range medicalScale.Factors {
		result[f.Code] = f.MaxScore
	}
	return result
}

func emptyHighRiskFactorsResult(assessmentID uint64) *HighRiskFactorsResult {
	return &HighRiskFactorsResult{
		AssessmentID:    assessmentID,
		HasHighRisk:     false,
		HighRiskFactors: nil,
		NeedsUrgentCare: false,
	}
}
