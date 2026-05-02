package assessment

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// ============= 领域模型到 DTO 的转换器 =============

// toAssessmentResult 将领域模型转换为 AssessmentResult
func toAssessmentResult(a *assessment.Assessment) (*AssessmentResult, error) {
	if a == nil {
		return nil, nil
	}

	orgID, err := safeconv.Int64ToUint64(a.OrgID())
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}

	result := &AssessmentResult{
		ID:                   a.ID().Uint64(),
		OrgID:                orgID,
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

	return result, nil
}

func assessmentRowToResult(row evaluationreadmodel.AssessmentRow) (*AssessmentResult, error) {
	orgID, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	return &AssessmentResult{
		ID:                   row.ID,
		OrgID:                orgID,
		TesteeID:             row.TesteeID,
		QuestionnaireCode:    row.QuestionnaireCode,
		QuestionnaireVersion: row.QuestionnaireVersion,
		AnswerSheetID:        row.AnswerSheetID,
		MedicalScaleID:       row.MedicalScaleID,
		MedicalScaleCode:     row.MedicalScaleCode,
		MedicalScaleName:     row.MedicalScaleName,
		OriginType:           row.OriginType,
		OriginID:             row.OriginID,
		Status:               row.Status,
		TotalScore:           row.TotalScore,
		RiskLevel:            row.RiskLevel,
		SubmittedAt:          row.SubmittedAt,
		InterpretedAt:        row.InterpretedAt,
		FailedAt:             row.FailedAt,
		FailureReason:        row.FailureReason,
	}, nil
}

func assessmentRowsToResults(rows []evaluationreadmodel.AssessmentRow) ([]*AssessmentResult, error) {
	results := make([]*AssessmentResult, 0, len(rows))
	for _, row := range rows {
		result, err := assessmentRowToResult(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
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
		CreatedAt:    r.CreatedAt(),
	}
}

func reportRowToResult(row evaluationreadmodel.ReportRow) *ReportResult {
	dimensions := make([]DimensionResult, 0, len(row.Dimensions))
	for _, d := range row.Dimensions {
		dimensions = append(dimensions, DimensionResult{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    d.MaxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
			Suggestion:  d.Suggestion,
		})
	}
	suggestions := make([]SuggestionDTO, 0, len(row.Suggestions))
	for _, s := range row.Suggestions {
		suggestions = append(suggestions, SuggestionDTO{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		})
	}
	return &ReportResult{
		AssessmentID: row.AssessmentID,
		ScaleName:    row.ScaleName,
		ScaleCode:    row.ScaleCode,
		TotalScore:   row.TotalScore,
		RiskLevel:    row.RiskLevel,
		Conclusion:   row.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		CreatedAt:    row.CreatedAt,
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
func toScoreResult(s *assessment.AssessmentScore, medicalScale *evaluationinput.ScaleSnapshot) *ScoreResult {
	if s == nil {
		return nil
	}

	// 构建因子 max_score 映射
	factorMaxScoreMap := make(map[string]*float64)
	if medicalScale != nil {
		for _, f := range medicalScale.Factors {
			factorMaxScoreMap[f.Code] = f.MaxScore
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

func scoreRowToResult(row *evaluationreadmodel.ScoreRow, medicalScale *evaluationinput.ScaleSnapshot) *ScoreResult {
	if row == nil {
		return nil
	}
	factorMaxScoreMap := make(map[string]*float64)
	if medicalScale != nil {
		for _, f := range medicalScale.Factors {
			factorMaxScoreMap[f.Code] = f.MaxScore
		}
	}
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
		return &HighRiskFactorsResult{
			AssessmentID:    assessmentID,
			HasHighRisk:     false,
			HighRiskFactors: nil,
			NeedsUrgentCare: false,
		}
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
		for _, f := range medicalScale.Factors {
			factorMaxScoreMap[f.Code] = f.MaxScore
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
