package grpcbridge

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

func formatAnswerSheetID(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func toDimensionInterpretResponses(outputs []DimensionInterpretOutput) []evaluation.DimensionInterpretResponse {
	if len(outputs) == 0 {
		return nil
	}
	dimensions := make([]evaluation.DimensionInterpretResponse, 0, len(outputs))
	for _, dim := range outputs {
		item := evaluation.DimensionInterpretResponse{
			FactorCode:  dim.FactorCode,
			FactorName:  dim.FactorName,
			RawScore:    dim.RawScore,
			MaxScore:    dim.MaxScore,
			RiskLevel:   dim.RiskLevel,
			Level:       toResultLevelResponse(dim.Level),
			Description: dim.Description,
			Suggestion:  dim.Suggestion,
		}
		for _, score := range dim.DerivedScores {
			item.DerivedScores = append(item.DerivedScores, *toScoreValueResponse(&score))
		}
		if dim.NormReference != nil {
			item.NormReference = &evaluation.NormReferenceResponse{ScoreKind: dim.NormReference.ScoreKind, Benchmark: dim.NormReference.Benchmark, TableVersion: dim.NormReference.TableVersion, FormVariant: dim.NormReference.FormVariant, MinAgeMonths: dim.NormReference.MinAgeMonths, MaxAgeMonths: dim.NormReference.MaxAgeMonths, Gender: dim.NormReference.Gender}
		}
		dimensions = append(dimensions, item)
	}
	return dimensions
}

func toFactorScoreResponses(result []FactorScoreOutput) []evaluation.FactorScoreResponse {
	scores := make([]evaluation.FactorScoreResponse, len(result))
	for i, score := range result {
		scores[i] = evaluation.FactorScoreResponse{
			FactorCode:   score.FactorCode,
			FactorName:   score.FactorName,
			RawScore:     score.RawScore,
			RiskLevel:    score.RiskLevel,
			IsTotalScore: score.IsTotalScore,
		}
	}
	return scores
}

func toSuggestionResponses(outputs []SuggestionOutput) []evaluation.SuggestionResponse {
	if len(outputs) == 0 {
		return nil
	}
	result := make([]evaluation.SuggestionResponse, len(outputs))
	for i, s := range outputs {
		result[i] = evaluation.SuggestionResponse{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		}
	}
	return result
}

func toTrendPointResponses(result []TrendPointOutput) []evaluation.TrendPointResponse {
	points := make([]evaluation.TrendPointResponse, len(result))
	for i, point := range result {
		points[i] = evaluation.TrendPointResponse{
			AssessmentID: strconv.FormatUint(point.AssessmentID, 10),
			Score:        point.Score,
			RiskLevel:    point.RiskLevel,
			CreatedAt:    point.CreatedAt,
		}
	}
	return points
}

func toAssessmentDetailResponse(detail *AssessmentDetailOutput) *evaluation.AssessmentDetailResponse {
	if detail == nil {
		return nil
	}
	return &evaluation.AssessmentDetailResponse{
		ID:                   strconv.FormatUint(detail.ID, 10),
		OrgID:                strconv.FormatUint(detail.OrgID, 10),
		TesteeID:             strconv.FormatUint(detail.TesteeID, 10),
		QuestionnaireCode:    detail.QuestionnaireCode,
		QuestionnaireVersion: detail.QuestionnaireVersion,
		AnswerSheetID:        formatAnswerSheetID(detail.AnswerSheetID),
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

func toAssessmentSummaryResponse(summary AssessmentSummaryOutput) evaluation.AssessmentSummaryResponse {
	return evaluation.AssessmentSummaryResponse{
		ID:                   strconv.FormatUint(summary.ID, 10),
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		AnswerSheetID:        formatAnswerSheetID(summary.AnswerSheetID),
		Model:                toModelIdentityResponse(summary.Model),
		PrimaryScore:         toScoreValueResponse(summary.PrimaryScore),
		Level:                toResultLevelResponse(summary.Level),
		OriginType:           summary.OriginType,
		Status:               summary.Status,
		SubmittedAt:          summary.SubmittedAt,
		InterpretedAt:        summary.InterpretedAt,
	}
}

func toListAssessmentsResponse(resp *ListAssessmentsOutput) *evaluation.ListAssessmentsResponse {
	if resp == nil {
		return nil
	}
	items := make([]evaluation.AssessmentSummaryResponse, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, toAssessmentSummaryResponse(item))
	}
	return &evaluation.ListAssessmentsResponse{
		Items:      items,
		Total:      resp.Total,
		Page:       resp.Page,
		PageSize:   resp.PageSize,
		TotalPages: resp.TotalPages,
	}
}

func toAssessmentReportResponse(report *AssessmentReportOutput) *evaluation.AssessmentReportResponse {
	if report == nil {
		return nil
	}
	return &evaluation.AssessmentReportResponse{
		AssessmentID: strconv.FormatUint(report.AssessmentID, 10),
		Model:        toModelIdentityResponse(report.Model),
		PrimaryScore: toScoreValueResponse(report.PrimaryScore),
		Level:        toResultLevelResponse(report.Level),
		Conclusion:   report.Conclusion,
		Dimensions:   toDimensionInterpretResponses(report.Dimensions),
		Suggestions:  toSuggestionResponses(report.Suggestions),
		ModelExtra:   toModelExtraResponse(report.ModelExtra),
		CreatedAt:    report.CreatedAt,
	}
}

func toModelIdentityResponse(model ModelIdentityOutput) evaluation.ModelIdentityResponse {
	return evaluation.ModelIdentityResponse{
		Kind:            model.Kind,
		SubKind:         model.SubKind,
		Algorithm:       model.Algorithm,
		Code:            model.Code,
		Version:         model.Version,
		Title:           model.Title,
		ProductChannel:  model.ProductChannel,
		AlgorithmFamily: model.AlgorithmFamily,
		DecisionKind:    model.DecisionKind,
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
