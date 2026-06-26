package evaluation

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"

func AssessmentDetailToV2(detail *AssessmentDetailResponse) *AssessmentDetailV2Response {
	if detail == nil {
		return nil
	}
	model, primary, level := legacyAssessmentOutcome(detail.ScaleCode, detail.ScaleName, detail.TotalScore, detail.RiskLevel)
	return &AssessmentDetailV2Response{
		ID:                   detail.ID,
		OrgID:                detail.OrgID,
		TesteeID:             detail.TesteeID,
		QuestionnaireCode:    detail.QuestionnaireCode,
		QuestionnaireVersion: detail.QuestionnaireVersion,
		AnswerSheetID:        detail.AnswerSheetID,
		Model:                model,
		PrimaryScore:         primary,
		Level:                level,
		OriginType:           detail.OriginType,
		OriginID:             detail.OriginID,
		Status:               detail.Status,
		CreatedAt:            detail.CreatedAt,
		SubmittedAt:          detail.SubmittedAt,
		InterpretedAt:        detail.InterpretedAt,
		FailedAt:             detail.FailedAt,
		FailureReason:        detail.FailureReason,
	}
}

func AssessmentSummaryToV2(summary AssessmentSummaryResponse) AssessmentSummaryV2Response {
	model, primary, level := legacyAssessmentOutcome(summary.ScaleCode, summary.ScaleName, summary.TotalScore, summary.RiskLevel)
	return AssessmentSummaryV2Response{
		ID:                   summary.ID,
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		AnswerSheetID:        summary.AnswerSheetID,
		Model:                model,
		PrimaryScore:         primary,
		Level:                level,
		OriginType:           summary.OriginType,
		Status:               summary.Status,
		CreatedAt:            summary.CreatedAt,
		SubmittedAt:          summary.SubmittedAt,
		InterpretedAt:        summary.InterpretedAt,
	}
}

func ListAssessmentsToV2(resp *ListAssessmentsResponse) *ListAssessmentsV2Response {
	if resp == nil {
		return nil
	}
	items := make([]AssessmentSummaryV2Response, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, AssessmentSummaryToV2(item))
	}
	return &ListAssessmentsV2Response{
		Items:      items,
		Total:      resp.Total,
		Page:       resp.Page,
		PageSize:   resp.PageSize,
		TotalPages: resp.TotalPages,
	}
}

func AssessmentReportToV2(report *AssessmentReportResponse) *AssessmentReportV2Response {
	if report == nil {
		return nil
	}
	model, primary, level := legacyAssessmentOutcome(report.ScaleCode, report.ScaleName, report.TotalScore, report.RiskLevel)
	return &AssessmentReportV2Response{
		AssessmentID: report.AssessmentID,
		Model:        model,
		PrimaryScore: primary,
		Level:        level,
		Conclusion:   report.Conclusion,
		Dimensions:   report.Dimensions,
		Suggestions:  report.Suggestions,
		CreatedAt:    report.CreatedAt,
	}
}

func legacyAssessmentOutcome(scaleCode, scaleName string, totalScore float64, riskLevel string) (ModelIdentityResponse, *ScoreValueResponse, *ResultLevelResponse) {
	model := ModelIdentityResponse{
		Kind:      "scale",
		Algorithm: "scale_default",
		Code:      scaleCode,
		Title:     scaleName,
	}
	var primary *ScoreValueResponse
	if totalScore != 0 {
		primary = &ScoreValueResponse{Kind: domainreport.ScoreKindRawTotal, Value: totalScore}
	}
	var level *ResultLevelResponse
	if riskLevel != "" && domainreport.IsRiskLevelCode(riskLevel) {
		if lv := domainreport.LevelFromRisk(domainreport.RiskLevel(riskLevel)); lv != nil {
			level = &ResultLevelResponse{Code: lv.Code, Label: lv.Label, Severity: lv.Severity}
		}
	}
	return model, primary, level
}
