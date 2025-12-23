package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ============= 领域模型到 DTO 的转换器 =============

// toAssessmentResult 将领域模型转换为 AssessmentResult
func toAssessmentResult(a *assessment.Assessment) *AssessmentResult {
	if a == nil {
		return nil
	}

	result := &AssessmentResult{
		ID:                   a.ID().Uint64(),
		OrgID:                uint64(a.OrgID()),
		TesteeID:             a.TesteeID().Uint64(),
		QuestionnaireCode:    a.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: a.QuestionnaireRef().Version(),
		AnswerSheetID:        a.AnswerSheetRef().ID().Uint64(),
		OriginType:           a.Origin().Type().String(),
		Status:               a.Status().String(),
	}

	// 量表引用（可选）
	if scaleRef := a.MedicalScaleRef(); scaleRef != nil {
		scaleID := scaleRef.ID().Uint64()
		scaleCode := scaleRef.Code().String()
		scaleName := scaleRef.Name()
		result.MedicalScaleID = &scaleID
		result.MedicalScaleCode = &scaleCode
		result.MedicalScaleName = &scaleName
	}

	// 来源ID（可选）
	if originID := a.Origin().ID(); originID != nil {
		result.OriginID = originID
	}

	// 总分（可选）
	if totalScore := a.TotalScore(); totalScore != nil {
		result.TotalScore = totalScore
	}

	// 风险等级（可选）
	if riskLevel := a.RiskLevel(); riskLevel != nil {
		rl := string(*riskLevel)
		result.RiskLevel = &rl
	}

	// 时间戳
	if submittedAt := a.SubmittedAt(); submittedAt != nil {
		result.SubmittedAt = submittedAt
	}
	if interpretedAt := a.InterpretedAt(); interpretedAt != nil {
		result.InterpretedAt = interpretedAt
	}
	if failedAt := a.FailedAt(); failedAt != nil {
		result.FailedAt = failedAt
	}
	if failureReason := a.FailureReason(); failureReason != nil {
		result.FailureReason = failureReason
	}

	return result
}

// toReportResult 将领域模型转换为 ReportResult
func toReportResult(r *report.InterpretReport) *ReportResult {
	if r == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]DimensionResult, len(r.Dimensions()))
	for i, d := range r.Dimensions() {
		dimensions[i] = DimensionResult{
			FactorCode:  string(d.FactorCode()),
			FactorName:  d.FactorName(),
			RawScore:    d.RawScore(),
			MaxScore:    d.MaxScore(),
			RiskLevel:   string(d.RiskLevel()),
			Description: d.Description(),
			Suggestion:  d.Suggestion(),
		}
	}

	return &ReportResult{
		AssessmentID: r.ID().Uint64(),
		ScaleName:    r.ScaleName(),
		ScaleCode:    r.ScaleCode(),
		TotalScore:   r.TotalScore(),
		RiskLevel:    string(r.RiskLevel()),
		Conclusion:   r.Conclusion(),
		Dimensions:   dimensions,
		Suggestions:  toSuggestionDTOs(r.Suggestions()),
	}
}

func toSuggestionDTOs(items []report.Suggestion) []SuggestionDTO {
	if len(items) == 0 {
		return nil
	}
	result := make([]SuggestionDTO, len(items))
	for i, s := range items {
		var fc *string
		if s.FactorCode != nil {
			code := s.FactorCode.String()
			fc = &code
		}
		result[i] = SuggestionDTO{
			Category:   string(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}

// toScoreResult 将领域模型转换为 ScoreResult
func toScoreResult(s *assessment.AssessmentScore, medicalScale *scale.MedicalScale) *ScoreResult {
	if s == nil {
		return nil
	}

	// 构建因子 max_score 映射
	factorMaxScoreMap := make(map[string]*float64)
	if medicalScale != nil {
		factors := medicalScale.GetFactors()
		for _, f := range factors {
			factorMaxScoreMap[string(f.GetCode())] = f.GetMaxScore()
		}
	}

	// 转换因子得分列表
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

// toHighRiskFactorsResult 转换高风险因子结果
func toHighRiskFactorsResult(assessmentID uint64, s *assessment.AssessmentScore, medicalScale *scale.MedicalScale) *HighRiskFactorsResult {
	if s == nil {
		return &HighRiskFactorsResult{
			AssessmentID:    assessmentID,
			HasHighRisk:     false,
			HighRiskFactors: nil,
			NeedsUrgentCare: false,
		}
	}

	// 构建因子 max_score 映射
	factorMaxScoreMap := make(map[string]*float64)
	if medicalScale != nil {
		factors := medicalScale.GetFactors()
		for _, f := range factors {
			factorMaxScoreMap[string(f.GetCode())] = f.GetMaxScore()
		}
	}

	// 获取高风险因子
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

	// 判断是否需要紧急关注（严重风险或多个高风险因子）
	needsUrgentCare := s.RiskLevel() == assessment.RiskLevelSevere || len(highRiskFactors) >= 3

	return &HighRiskFactorsResult{
		AssessmentID:    assessmentID,
		HasHighRisk:     len(highRiskFactors) > 0,
		HighRiskFactors: factorResults,
		NeedsUrgentCare: needsUrgentCare,
	}
}
