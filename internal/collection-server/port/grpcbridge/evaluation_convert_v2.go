package grpcbridge

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

func toAssessmentDetailV2Response(detail *AssessmentDetailV2Output) *evaluation.AssessmentDetailV2Response {
	if detail == nil {
		return nil
	}
	return &evaluation.AssessmentDetailV2Response{
		ID:                   strconv.FormatUint(detail.ID, 10),
		OrgID:                strconv.FormatUint(detail.OrgID, 10),
		TesteeID:             strconv.FormatUint(detail.TesteeID, 10),
		QuestionnaireCode:    detail.QuestionnaireCode,
		QuestionnaireVersion: detail.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(detail.AnswerSheetID, 10),
		Model:                toModelIdentityResponse(detail.Model),
		PrimaryScore:         toScoreValueResponse(detail.PrimaryScore),
		Level:                toResultLevelResponse(detail.Level),
		OriginType:           detail.OriginType,
		OriginID:             detail.OriginID,
		Status:               detail.Status,
		SubmittedAt:          detail.SubmittedAt,
		InterpretedAt:        detail.InterpretedAt,
		FailedAt:             detail.FailedAt,
		FailureReason:        detail.FailureReason,
	}
}

func toAssessmentSummaryV2Response(summary AssessmentSummaryV2Output) evaluation.AssessmentSummaryV2Response {
	return evaluation.AssessmentSummaryV2Response{
		ID:                   strconv.FormatUint(summary.ID, 10),
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(summary.AnswerSheetID, 10),
		Model:                toModelIdentityResponse(summary.Model),
		PrimaryScore:         toScoreValueResponse(summary.PrimaryScore),
		Level:                toResultLevelResponse(summary.Level),
		OriginType:           summary.OriginType,
		Status:               summary.Status,
		SubmittedAt:          summary.SubmittedAt,
		InterpretedAt:        summary.InterpretedAt,
	}
}

func toListAssessmentsV2Response(resp *ListAssessmentsV2Output) *evaluation.ListAssessmentsV2Response {
	if resp == nil {
		return nil
	}
	items := make([]evaluation.AssessmentSummaryV2Response, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, toAssessmentSummaryV2Response(item))
	}
	return &evaluation.ListAssessmentsV2Response{
		Items:      items,
		Total:      resp.Total,
		Page:       resp.Page,
		PageSize:   resp.PageSize,
		TotalPages: resp.TotalPages,
	}
}

func toAssessmentReportV2Response(report *AssessmentReportV2Output) *evaluation.AssessmentReportV2Response {
	if report == nil {
		return nil
	}
	dimensions := make([]evaluation.DimensionInterpretResponse, 0, len(report.Dimensions))
	for _, dim := range report.Dimensions {
		dimensions = append(dimensions, evaluation.DimensionInterpretResponse{
			FactorCode:  dim.FactorCode,
			FactorName:  dim.FactorName,
			RawScore:    dim.RawScore,
			MaxScore:    dim.MaxScore,
			RiskLevel:   dim.RiskLevel,
			Description: dim.Description,
			Suggestion:  dim.Suggestion,
		})
	}
	suggestions := make([]evaluation.SuggestionResponse, 0, len(report.Suggestions))
	for _, item := range report.Suggestions {
		suggestions = append(suggestions, evaluation.SuggestionResponse{
			Category:   item.Category,
			Content:    item.Content,
			FactorCode: item.FactorCode,
		})
	}
	return &evaluation.AssessmentReportV2Response{
		AssessmentID: strconv.FormatUint(report.AssessmentID, 10),
		Model:        toModelIdentityResponse(report.Model),
		PrimaryScore: toScoreValueResponse(report.PrimaryScore),
		Level:        toResultLevelResponse(report.Level),
		Conclusion:   report.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   toModelExtraResponse(report.ModelExtra),
		CreatedAt:    report.CreatedAt,
	}
}

func toModelIdentityResponse(model ModelIdentityOutput) evaluation.ModelIdentityResponse {
	return evaluation.ModelIdentityResponse{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func toScoreValueResponse(score *ScoreValueOutput) *evaluation.ScoreValueResponse {
	if score == nil {
		return nil
	}
	return &evaluation.ScoreValueResponse{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func toResultLevelResponse(level *ResultLevelOutput) *evaluation.ResultLevelResponse {
	if level == nil {
		return nil
	}
	return &evaluation.ResultLevelResponse{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func toModelExtraResponse(extra *ModelExtraOutput) *evaluation.ModelExtraResponse {
	if extra == nil {
		return nil
	}
	resp := &evaluation.ModelExtraResponse{
		Kind:           extra.Kind,
		TypeCode:       extra.TypeCode,
		TypeName:       extra.TypeName,
		OneLiner:       extra.OneLiner,
		ImageURL:       extra.ImageURL,
		MatchPercent:   extra.MatchPercent,
		IsSpecial:      extra.IsSpecial,
		SpecialTrigger: extra.SpecialTrigger,
		Commentary:     extra.Commentary,
	}
	if extra.Rarity != nil {
		resp.Rarity = &evaluation.ModelRarityResponse{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return resp
}
