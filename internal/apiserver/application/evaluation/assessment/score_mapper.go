package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// toScoreResult 将领域模型转换为 ScoreResult
func toScoreResult(s *assessment.AssessmentScore, medicalScale *evaluationinput.ScaleSnapshot) *ScoreResult {
	if s == nil {
		return nil
	}

	factorMaxScoreMap := factorMaxScores(medicalScale)
	factorScores := make([]FactorScoreResult, len(s.FactorScores()))
	for i, fs := range s.FactorScores() {
		factorCode := string(fs.FactorCode())
		factorScores[i] = FactorScoreResult{
			FactorCode:   factorCode,
			FactorName:   fs.FactorName(),
			RawScore:     fs.RawScore(),
			MaxScore:     factorMaxScoreMap[factorCode],
			RiskLevel:    string(fs.RiskLevel()),
			IsTotalScore: fs.IsTotalScore(),
		}
	}

	return &ScoreResult{
		AssessmentID: s.AssessmentID().Uint64(),
		TotalScore:   s.TotalScore(),
		RiskLevel:    string(s.RiskLevel()),
		FactorScores: factorScores,
	}
}

func scoreRowToResult(row *evaluationreadmodel.ScoreRow, medicalScale *evaluationinput.ScaleSnapshot) *ScoreResult {
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
			Conclusion:   fs.Conclusion,
			Suggestion:   fs.Suggestion,
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

func highRiskFactorsResultFromScoreRow(assessmentID uint64, row *evaluationreadmodel.ScoreRow, medicalScale *evaluationinput.ScaleSnapshot) *HighRiskFactorsResult {
	if row == nil {
		return emptyHighRiskFactorsResult(assessmentID)
	}

	scoreResult := scoreRowToResult(row, medicalScale)
	highRiskFactors := make([]FactorScoreResult, 0)
	for _, fs := range scoreResult.FactorScores {
		if fs.RiskLevel == string(assessment.RiskLevelHigh) || fs.RiskLevel == string(assessment.RiskLevelSevere) {
			highRiskFactors = append(highRiskFactors, fs)
		}
	}
	needsUrgentCare := row.RiskLevel == string(assessment.RiskLevelSevere) || len(highRiskFactors) >= 3
	return &HighRiskFactorsResult{
		AssessmentID:    assessmentID,
		HasHighRisk:     len(highRiskFactors) > 0,
		HighRiskFactors: highRiskFactors,
		NeedsUrgentCare: needsUrgentCare,
	}
}

// toHighRiskFactorsResult 转换高风险因子结果
func toHighRiskFactorsResult(assessmentID uint64, s *assessment.AssessmentScore, medicalScale *evaluationinput.ScaleSnapshot) *HighRiskFactorsResult {
	if s == nil {
		return emptyHighRiskFactorsResult(assessmentID)
	}

	factorMaxScoreMap := factorMaxScores(medicalScale)
	highRiskFactors := s.GetHighRiskFactors()
	factorResults := make([]FactorScoreResult, len(highRiskFactors))
	for i, fs := range highRiskFactors {
		factorCode := string(fs.FactorCode())
		factorResults[i] = FactorScoreResult{
			FactorCode:   factorCode,
			FactorName:   fs.FactorName(),
			RawScore:     fs.RawScore(),
			MaxScore:     factorMaxScoreMap[factorCode],
			RiskLevel:    string(fs.RiskLevel()),
			IsTotalScore: fs.IsTotalScore(),
		}
	}

	needsUrgentCare := s.RiskLevel() == assessment.RiskLevelSevere || len(highRiskFactors) >= 3
	return &HighRiskFactorsResult{
		AssessmentID:    assessmentID,
		HasHighRisk:     len(highRiskFactors) > 0,
		HighRiskFactors: factorResults,
		NeedsUrgentCare: needsUrgentCare,
	}
}

func factorMaxScores(medicalScale *evaluationinput.ScaleSnapshot) map[string]*float64 {
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
