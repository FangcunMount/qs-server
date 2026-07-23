package service

import (
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	interpretationParticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
)

func toEvaluationProtoModelIdentity(model evaluationtestee.ModelIdentity) *evaluationpb.ModelIdentity {
	return &evaluationpb.ModelIdentity{
		Kind: model.Kind, Algorithm: model.Algorithm, Code: model.Code, Version: model.Version, Title: model.Title, DecisionKind: model.DecisionKind,
	}
}

func toEvaluationProtoScoreValue(score *evaluationtestee.ScoreValue) *evaluationpb.ScoreValue {
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

func toEvaluationProtoResultLevel(level *evaluationtestee.ResultLevel) *evaluationpb.ResultLevel {
	if level == nil {
		return nil
	}
	return &evaluationpb.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func toProtoAssessmentDetailFromOutcome(result *evaluationtestee.Assessment) *evaluationpb.AssessmentDetail {
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
	if result.OriginID != nil {
		detail.OriginId = *result.OriginID
	}
	if result.SubmittedAt != nil {
		detail.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailedAt != nil {
		detail.FailedAt = result.FailedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailureReason != nil {
		detail.FailureReason = *result.FailureReason
	}
	return detail
}

func toProtoAssessmentSummaryFromOutcome(result *evaluationtestee.Assessment) *evaluationpb.AssessmentSummary {
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
	if result.SubmittedAt != nil {
		summary.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	return summary
}

func toProtoParticipantReport(result *interpretationParticipant.Report) *interpretationpb.AssessmentReport {
	if result == nil {
		return nil
	}
	report := &interpretationpb.AssessmentReport{
		AssessmentId: result.AssessmentID,
		Model:        &evaluationpb.ModelIdentity{Kind: result.Model.Kind, Algorithm: result.Model.Algorithm, Code: result.Model.Code, Version: result.Model.Version, Title: result.Model.Title},
		Conclusion:   result.Conclusion, CreatedAt: result.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if result.PrimaryScore != nil {
		report.PrimaryScore = &evaluationpb.ScoreValue{Kind: result.PrimaryScore.Kind, Value: result.PrimaryScore.Value, Label: result.PrimaryScore.Label, Max: result.PrimaryScore.Max}
	}
	if result.Level != nil {
		report.Level = &evaluationpb.ResultLevel{Code: result.Level.Code, Label: result.Level.Label, Severity: result.Level.Severity}
	}
	for _, d := range result.Dimensions {
		dimension := &interpretationpb.DimensionInterpret{FactorCode: d.FactorCode, FactorName: d.FactorName, RawScore: d.RawScore, MaxScore: derefFloat64(d.MaxScore), RiskLevel: d.RiskLevel, Description: d.Description, Suggestion: d.Suggestion}
		for _, score := range d.DerivedScores {
			dimension.DerivedScores = append(dimension.DerivedScores, &evaluationpb.ScoreValue{Kind: score.Kind, Value: score.Value, Label: score.Label, Max: score.Max})
		}
		if d.Level != nil {
			dimension.Level = &evaluationpb.ResultLevel{Code: d.Level.Code, Label: d.Level.Label, Severity: d.Level.Severity}
		}
		if d.NormReference != nil {
			dimension.NormReference = &interpretationpb.NormReference{ScoreKind: d.NormReference.ScoreKind, Benchmark: d.NormReference.Benchmark, TableVersion: d.NormReference.TableVersion, FormVariant: d.NormReference.FormVariant, MinAgeMonths: int32(d.NormReference.MinAgeMonths), MaxAgeMonths: int32(d.NormReference.MaxAgeMonths), Gender: d.NormReference.Gender}
		}
		report.Dimensions = append(report.Dimensions, dimension)
	}
	for _, s := range result.Suggestions {
		item := &interpretationpb.Suggestion{Category: s.Category, Content: s.Content}
		if s.FactorCode != nil {
			item.FactorCode = *s.FactorCode
		}
		report.Suggestions = append(report.Suggestions, item)
	}
	if result.ModelExtra != nil {
		report.ModelExtra = &interpretationpb.ModelExtra{Kind: result.ModelExtra.Kind, TypeCode: result.ModelExtra.TypeCode, TypeName: result.ModelExtra.TypeName, OneLiner: result.ModelExtra.OneLiner, ImageUrl: result.ModelExtra.ImageURL, MatchPercent: result.ModelExtra.MatchPercent, IsSpecial: result.ModelExtra.IsSpecial, SpecialTrigger: result.ModelExtra.SpecialTrigger, Commentary: result.ModelExtra.Commentary}
		if result.ModelExtra.Rarity != nil {
			report.ModelExtra.Rarity = &interpretationpb.ModelRarity{Percent: result.ModelExtra.Rarity.Percent, Label: result.ModelExtra.Rarity.Label, OneInX: int32(result.ModelExtra.Rarity.OneInX)}
		}
	}
	return report
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
