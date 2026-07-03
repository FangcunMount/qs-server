package grpcbridge

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

func toAssessmentDetailResponse(result *AssessmentDetailOutput) *evaluation.AssessmentDetailResponse {
	if result == nil {
		return nil
	}
	answerSheetID := ""
	if result.AnswerSheetID != 0 {
		answerSheetID = strconv.FormatUint(result.AnswerSheetID, 10)
	}
	return &evaluation.AssessmentDetailResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		OrgID:                strconv.FormatUint(result.OrgID, 10),
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        answerSheetID,
		ScaleCode:            result.ScaleCode,
		ScaleName:            result.ScaleName,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		TotalScore:           result.TotalScore,
		RiskLevel:            result.RiskLevel,
		CreatedAt:            result.CreatedAt,
		SubmittedAt:          result.SubmittedAt,
		InterpretedAt:        result.InterpretedAt,
		FailedAt:             result.FailedAt,
		FailureReason:        result.FailureReason,
	}
}

func toListAssessmentsResponse(result *ListAssessmentsOutput) *evaluation.ListAssessmentsResponse {
	if result == nil {
		return nil
	}
	items := make([]evaluation.AssessmentSummaryResponse, len(result.Items))
	for i, item := range result.Items {
		answerSheetID := ""
		if item.AnswerSheetID != 0 {
			answerSheetID = strconv.FormatUint(item.AnswerSheetID, 10)
		}
		items[i] = evaluation.AssessmentSummaryResponse{
			ID:                   strconv.FormatUint(item.ID, 10),
			QuestionnaireCode:    item.QuestionnaireCode,
			QuestionnaireVersion: item.QuestionnaireVersion,
			AnswerSheetID:        answerSheetID,
			ScaleCode:            item.ScaleCode,
			ScaleName:            item.ScaleName,
			OriginType:           item.OriginType,
			Status:               item.Status,
			TotalScore:           item.TotalScore,
			RiskLevel:            item.RiskLevel,
			CreatedAt:            item.CreatedAt,
			SubmittedAt:          item.SubmittedAt,
			InterpretedAt:        item.InterpretedAt,
		}
	}
	return &evaluation.ListAssessmentsResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

func toFactorScoreResponses(result []FactorScoreOutput) []evaluation.FactorScoreResponse {
	scores := make([]evaluation.FactorScoreResponse, len(result))
	for i, score := range result {
		scores[i] = evaluation.FactorScoreResponse{
			FactorCode:   score.FactorCode,
			FactorName:   score.FactorName,
			RawScore:     score.RawScore,
			RiskLevel:    score.RiskLevel,
			Conclusion:   score.Conclusion,
			Suggestion:   score.Suggestion,
			IsTotalScore: score.IsTotalScore,
		}
	}
	return scores
}

func toAssessmentReportResponse(result *AssessmentReportOutput) *evaluation.AssessmentReportResponse {
	if result == nil {
		return nil
	}
	dimensions := make([]evaluation.DimensionInterpretResponse, 0, len(result.Dimensions))
	for _, dim := range result.Dimensions {
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
	return &evaluation.AssessmentReportResponse{
		AssessmentID: strconv.FormatUint(result.AssessmentID, 10),
		ScaleCode:    result.ScaleCode,
		ScaleName:    result.ScaleName,
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  toSuggestionResponses(result.Suggestions),
		CreatedAt:    result.CreatedAt,
	}
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
