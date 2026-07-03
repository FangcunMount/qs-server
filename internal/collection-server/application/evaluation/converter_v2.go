package evaluation

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

func AssessmentDetailV2FromOutput(detail *grpcbridge.AssessmentDetailV2Output) *AssessmentDetailV2Response {
	if detail == nil {
		return nil
	}
	return &AssessmentDetailV2Response{
		ID:                   strconv.FormatUint(detail.ID, 10),
		OrgID:                strconv.FormatUint(detail.OrgID, 10),
		TesteeID:             strconv.FormatUint(detail.TesteeID, 10),
		QuestionnaireCode:    detail.QuestionnaireCode,
		QuestionnaireVersion: detail.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(detail.AnswerSheetID, 10),
		Model:                modelIdentityFromOutput(detail.Model),
		PrimaryScore:         scoreValueFromOutput(detail.PrimaryScore),
		Level:                resultLevelFromOutput(detail.Level),
		OriginType:           detail.OriginType,
		OriginID:             detail.OriginID,
		Status:               detail.Status,
		SubmittedAt:          detail.SubmittedAt,
		InterpretedAt:        detail.InterpretedAt,
		FailedAt:             detail.FailedAt,
		FailureReason:        detail.FailureReason,
	}
}

func AssessmentSummaryV2FromOutput(summary grpcbridge.AssessmentSummaryV2Output) AssessmentSummaryV2Response {
	return AssessmentSummaryV2Response{
		ID:                   strconv.FormatUint(summary.ID, 10),
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		AnswerSheetID:        strconv.FormatUint(summary.AnswerSheetID, 10),
		Model:                modelIdentityFromOutput(summary.Model),
		PrimaryScore:         scoreValueFromOutput(summary.PrimaryScore),
		Level:                resultLevelFromOutput(summary.Level),
		OriginType:           summary.OriginType,
		Status:               summary.Status,
		SubmittedAt:          summary.SubmittedAt,
		InterpretedAt:        summary.InterpretedAt,
	}
}

func ListAssessmentsV2FromOutput(resp *grpcbridge.ListAssessmentsV2Output) *ListAssessmentsV2Response {
	if resp == nil {
		return nil
	}
	items := make([]AssessmentSummaryV2Response, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, AssessmentSummaryV2FromOutput(item))
	}
	return &ListAssessmentsV2Response{
		Items:      items,
		Total:      resp.Total,
		Page:       resp.Page,
		PageSize:   resp.PageSize,
		TotalPages: resp.TotalPages,
	}
}

func AssessmentReportV2FromOutput(report *grpcbridge.AssessmentReportV2Output) *AssessmentReportV2Response {
	if report == nil {
		return nil
	}
	dimensions := make([]DimensionInterpretResponse, 0, len(report.Dimensions))
	for _, dim := range report.Dimensions {
		dimensions = append(dimensions, DimensionInterpretResponse{
			FactorCode:  dim.FactorCode,
			FactorName:  dim.FactorName,
			RawScore:    dim.RawScore,
			MaxScore:    dim.MaxScore,
			RiskLevel:   dim.RiskLevel,
			Description: dim.Description,
			Suggestion:  dim.Suggestion,
		})
	}
	suggestions := make([]SuggestionResponse, 0, len(report.Suggestions))
	for _, item := range report.Suggestions {
		suggestions = append(suggestions, SuggestionResponse{
			Category:   item.Category,
			Content:    item.Content,
			FactorCode: item.FactorCode,
		})
	}
	return &AssessmentReportV2Response{
		AssessmentID: strconv.FormatUint(report.AssessmentID, 10),
		Model:        modelIdentityFromOutput(report.Model),
		PrimaryScore: scoreValueFromOutput(report.PrimaryScore),
		Level:        resultLevelFromOutput(report.Level),
		Conclusion:   report.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   modelExtraFromOutput(report.ModelExtra),
		CreatedAt:    report.CreatedAt,
	}
}

func modelIdentityFromOutput(model grpcbridge.ModelIdentityOutput) ModelIdentityResponse {
	return ModelIdentityResponse{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}

func scoreValueFromOutput(score *grpcbridge.ScoreValueOutput) *ScoreValueResponse {
	if score == nil {
		return nil
	}
	return &ScoreValueResponse{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func resultLevelFromOutput(level *grpcbridge.ResultLevelOutput) *ResultLevelResponse {
	if level == nil {
		return nil
	}
	return &ResultLevelResponse{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func modelExtraFromOutput(extra *grpcbridge.ModelExtraOutput) *ModelExtraResponse {
	if extra == nil {
		return nil
	}
	resp := &ModelExtraResponse{
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
		resp.Rarity = &ModelRarityResponse{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return resp
}
