package service

import (
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
	internalpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
)

func toInternalOutcomeSummary(result *assessmentApp.AssessmentV2Result) *internalpb.OutcomeSummary {
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
	v2 := legacyAssessmentV2Result(result)
	return toInternalOutcomeSummary(v2)
}

func legacyAssessmentV2Result(result *assessmentApp.AssessmentResult) *assessmentApp.AssessmentV2Result {
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
	return &assessmentApp.AssessmentV2Result{
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

func toEvaluationProtoAssessmentDetailV2(result *assessmentApp.AssessmentV2Result) *evaluationpb.AssessmentDetailV2 {
	if result == nil {
		return nil
	}
	detail := &evaluationpb.AssessmentDetailV2{
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

func toEvaluationProtoAssessmentSummaryV2(result *assessmentApp.AssessmentV2Result) *evaluationpb.AssessmentSummaryV2 {
	if result == nil {
		return nil
	}
	summary := &evaluationpb.AssessmentSummaryV2{
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
	if result.SubmittedAt != nil {
		summary.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.InterpretedAt != nil {
		summary.InterpretedAt = result.InterpretedAt.Format("2006-01-02 15:04:05")
	}
	return summary
}

func toEvaluationProtoAssessmentReportV2(result *assessmentApp.ReportV2Result) *evaluationpb.AssessmentReportV2 {
	if result == nil {
		return nil
	}
	report := &evaluationpb.AssessmentReportV2{
		AssessmentId: result.AssessmentID,
		Model:        toEvaluationProtoModelIdentity(result.Model),
		PrimaryScore: toEvaluationProtoScoreValue(result.PrimaryScore),
		Level:        toEvaluationProtoResultLevel(result.Level),
		Conclusion:   result.Conclusion,
		CreatedAt:    result.CreatedAt.Format("2006-01-02 15:04:05"),
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
		report.ModelExtra = &evaluationpb.ModelExtra{TypeCode: result.ModelExtra.TypeCode}
	}
	return report
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
