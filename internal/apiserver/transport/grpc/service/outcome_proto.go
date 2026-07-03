package service

import (
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	internalpb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

func toInternalOutcomeSummary(result *assessmentApp.AssessmentOutcomeResult) *internalpb.OutcomeSummary {
	if result == nil {
		return nil
	}
	return &internalpb.OutcomeSummary{
		Model:        toInternalProtoModelIdentity(result.Model),
		PrimaryScore: toInternalProtoScoreValue(result.PrimaryScore),
		Level:        toInternalProtoResultLevel(result.Level),
	}
}

func toInternalProtoModelIdentity(model assessmentApp.ModelIdentityResult) *internalpb.ModelIdentity {
	return &internalpb.ModelIdentity{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func toInternalProtoScoreValue(score *assessmentApp.ScoreValueResult) *internalpb.ScoreValue {
	if score == nil {
		return nil
	}
	pbScore := &internalpb.ScoreValue{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
	}
	if score.Max != nil {
		pbScore.Max = score.Max
	}
	return pbScore
}

func toInternalProtoResultLevel(level *assessmentApp.ResultLevelResult) *internalpb.ResultLevel {
	if level == nil {
		return nil
	}
	return &internalpb.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func outcomeSummaryFromAssessmentResult(result *assessmentApp.AssessmentResult) *internalpb.OutcomeSummary {
	if result == nil {
		return nil
	}
	outcome := legacyAssessmentOutcomeResult(result)
	return toInternalOutcomeSummary(outcome)
}

func legacyAssessmentOutcomeResult(result *assessmentApp.AssessmentResult) *assessmentApp.AssessmentOutcomeResult {
	if result == nil {
		return nil
	}
	model := assessmentApp.ModelIdentityResult{Kind: "scale", Algorithm: "scale_default"}
	if result.MedicalScaleCode != nil {
		model.Code = *result.MedicalScaleCode
	}
	if result.MedicalScaleName != nil {
		model.Title = *result.MedicalScaleName
	}
	var primary *assessmentApp.ScoreValueResult
	if result.TotalScore != nil {
		primary = &assessmentApp.ScoreValueResult{Kind: domainreport.ScoreKindRawTotal, Value: *result.TotalScore}
	}
	var level *assessmentApp.ResultLevelResult
	if result.RiskLevel != nil && *result.RiskLevel != "" {
		if lv := domainreport.LevelFromRisk(domainreport.RiskLevel(*result.RiskLevel)); lv != nil {
			level = &assessmentApp.ResultLevelResult{Code: lv.Code, Label: lv.Label, Severity: lv.Severity}
		}
	}
	return &assessmentApp.AssessmentOutcomeResult{
		ID:                   result.ID,
		OrgID:                result.OrgID,
		TesteeID:             result.TesteeID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        result.AnswerSheetID,
		Model:                model,
		PrimaryScore:         primary,
		Level:                level,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}
}

func toEvaluationProtoModelIdentity(model assessmentApp.ModelIdentityResult) *evaluationpb.ModelIdentity {
	return &evaluationpb.ModelIdentity{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func toEvaluationProtoScoreValue(score *assessmentApp.ScoreValueResult) *evaluationpb.ScoreValue {
	if score == nil {
		return nil
	}
	pbScore := &evaluationpb.ScoreValue{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
	}
	if score.Max != nil {
		pbScore.Max = score.Max
	}
	return pbScore
}

func toEvaluationProtoResultLevel(level *assessmentApp.ResultLevelResult) *evaluationpb.ResultLevel {
	if level == nil {
		return nil
	}
	return &evaluationpb.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func toProtoAssessmentDetailFromOutcome(result *assessmentApp.AssessmentOutcomeResult) *evaluationpb.AssessmentDetail {
	if result == nil {
		return nil
	}
	detail := &evaluationpb.AssessmentDetail{
		Id:                   result.ID,
		OrgId:                result.OrgID,
		TesteeId:             result.TesteeID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetId:        result.AnswerSheetID,
		Model:                toEvaluationProtoModelIdentity(result.Model),
		PrimaryScore:         toEvaluationProtoScoreValue(result.PrimaryScore),
		Level:                toEvaluationProtoResultLevel(result.Level),
		OriginType:           result.OriginType,
		Status:               result.Status,
	}
	fillLegacyAssessmentDetailFields(detail, result)
	if result.OriginID != nil {
		detail.OriginId = *result.OriginID
	}
	if result.SubmittedAt != nil {
		detail.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.InterpretedAt != nil {
		detail.InterpretedAt = result.InterpretedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailedAt != nil {
		detail.FailedAt = result.FailedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailureReason != nil {
		detail.FailureReason = *result.FailureReason
	}
	return detail
}

func toProtoAssessmentSummaryFromOutcome(result *assessmentApp.AssessmentOutcomeResult) *evaluationpb.AssessmentSummary {
	if result == nil {
		return nil
	}
	summary := &evaluationpb.AssessmentSummary{
		Id:                   result.ID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetId:        result.AnswerSheetID,
		Model:                toEvaluationProtoModelIdentity(result.Model),
		PrimaryScore:         toEvaluationProtoScoreValue(result.PrimaryScore),
		Level:                toEvaluationProtoResultLevel(result.Level),
		OriginType:           result.OriginType,
		Status:               result.Status,
	}
	fillLegacyAssessmentSummaryFields(summary, result)
	if result.SubmittedAt != nil {
		summary.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.InterpretedAt != nil {
		summary.InterpretedAt = result.InterpretedAt.Format("2006-01-02 15:04:05")
	}
	return summary
}

func toProtoAssessmentReportFromOutcome(result *assessmentApp.ReportOutcomeResult) *evaluationpb.AssessmentReport {
	if result == nil {
		return nil
	}
	report := &evaluationpb.AssessmentReport{
		AssessmentId: result.AssessmentID,
		Model:        toEvaluationProtoModelIdentity(result.Model),
		PrimaryScore: toEvaluationProtoScoreValue(result.PrimaryScore),
		Level:        toEvaluationProtoResultLevel(result.Level),
		Conclusion:   result.Conclusion,
		CreatedAt:    result.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if result.Model.Code != "" {
		report.ScaleCode = result.Model.Code
	}
	if result.Model.Title != "" {
		report.ScaleName = result.Model.Title
	}
	if result.PrimaryScore != nil {
		report.TotalScore = result.PrimaryScore.Value
	}
	if result.Level != nil {
		report.RiskLevel = resultLevelToRiskLevel(result.Level)
	}
	for _, d := range result.Dimensions {
		report.Dimensions = append(report.Dimensions, &evaluationpb.DimensionInterpret{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    derefFloat64(d.MaxScore),
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
			Suggestion:  d.Suggestion,
		})
	}
	for _, s := range result.Suggestions {
		item := &evaluationpb.Suggestion{Category: s.Category, Content: s.Content}
		if s.FactorCode != nil {
			item.FactorCode = *s.FactorCode
		}
		report.Suggestions = append(report.Suggestions, item)
	}
	if result.ModelExtra != nil {
		report.ModelExtra = toProtoModelExtra(result.ModelExtra)
	}
	return report
}

func fillLegacyAssessmentDetailFields(detail *evaluationpb.AssessmentDetail, result *assessmentApp.AssessmentOutcomeResult) {
	if detail == nil || result == nil {
		return
	}
	if result.Model.Code != "" {
		detail.ScaleCode = result.Model.Code
	}
	if result.Model.Title != "" {
		detail.ScaleName = result.Model.Title
	}
	if result.PrimaryScore != nil {
		detail.TotalScore = result.PrimaryScore.Value
	}
	if result.Level != nil {
		detail.RiskLevel = resultLevelToRiskLevel(result.Level)
	}
}

func fillLegacyAssessmentSummaryFields(summary *evaluationpb.AssessmentSummary, result *assessmentApp.AssessmentOutcomeResult) {
	if summary == nil || result == nil {
		return
	}
	if result.Model.Code != "" {
		summary.ScaleCode = result.Model.Code
	}
	if result.Model.Title != "" {
		summary.ScaleName = result.Model.Title
	}
	if result.PrimaryScore != nil {
		summary.TotalScore = result.PrimaryScore.Value
	}
	if result.Level != nil {
		summary.RiskLevel = resultLevelToRiskLevel(result.Level)
	}
}

func resultLevelToRiskLevel(level *assessmentApp.ResultLevelResult) string {
	if level == nil {
		return ""
	}
	return level.Code
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
