package evaluation

const personalityModelKind = "personality"

var legacyRiskLevelCodes = map[string]struct{}{
	"none": {}, "low": {}, "medium": {}, "high": {}, "severe": {},
}

func isLegacyRiskLevelCode(code string) bool {
	_, ok := legacyRiskLevelCodes[code]
	return ok
}

func legacyScaleCode(model ModelIdentityResponse) string {
	if model.Kind == personalityModelKind {
		return ""
	}
	return model.Code
}

func legacyScaleName(model ModelIdentityResponse) string {
	if model.Kind == personalityModelKind {
		return ""
	}
	return model.Title
}

func LegacyTotalScore(score *ScoreValueResponse) float64 {
	if score == nil {
		return 0
	}
	return score.Value
}

func LegacyRiskLevel(level *ResultLevelResponse) string {
	if level == nil {
		return ""
	}
	if isLegacyRiskLevelCode(level.Code) {
		return level.Code
	}
	if isLegacyRiskLevelCode(level.Severity) {
		return level.Severity
	}
	return ""
}

// DetailToLegacy projects outcome assessment detail onto the legacy REST shape.
func DetailToLegacy(detail *AssessmentDetailResponse) *LegacyAssessmentDetailResponse {
	if detail == nil {
		return nil
	}
	return &LegacyAssessmentDetailResponse{
		ID:                   detail.ID,
		OrgID:                detail.OrgID,
		TesteeID:             detail.TesteeID,
		QuestionnaireCode:    detail.QuestionnaireCode,
		QuestionnaireVersion: detail.QuestionnaireVersion,
		AnswerSheetID:        detail.AnswerSheetID,
		ScaleCode:            legacyScaleCode(detail.Model),
		ScaleName:            legacyScaleName(detail.Model),
		OriginType:           detail.OriginType,
		OriginID:             detail.OriginID,
		Status:               detail.Status,
		TotalScore:           LegacyTotalScore(detail.PrimaryScore),
		RiskLevel:            LegacyRiskLevel(detail.Level),
		CreatedAt:            detail.CreatedAt,
		SubmittedAt:          detail.SubmittedAt,
		InterpretedAt:        detail.InterpretedAt,
		FailedAt:             detail.FailedAt,
		FailureReason:        detail.FailureReason,
	}
}

// SummaryToLegacy projects an outcome list item onto the legacy REST shape.
func SummaryToLegacy(summary AssessmentSummaryResponse) LegacyAssessmentSummaryResponse {
	return LegacyAssessmentSummaryResponse{
		ID:                   summary.ID,
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		AnswerSheetID:        summary.AnswerSheetID,
		ScaleCode:            legacyScaleCode(summary.Model),
		ScaleName:            legacyScaleName(summary.Model),
		OriginType:           summary.OriginType,
		Status:               summary.Status,
		TotalScore:           LegacyTotalScore(summary.PrimaryScore),
		RiskLevel:            LegacyRiskLevel(summary.Level),
		CreatedAt:            summary.CreatedAt,
		SubmittedAt:          summary.SubmittedAt,
		InterpretedAt:        summary.InterpretedAt,
	}
}

// ListToLegacy projects a paginated outcome list onto the legacy REST shape.
func ListToLegacy(list *ListAssessmentsResponse) *LegacyListAssessmentsResponse {
	if list == nil {
		return nil
	}
	items := make([]LegacyAssessmentSummaryResponse, len(list.Items))
	for i, item := range list.Items {
		items[i] = SummaryToLegacy(item)
	}
	return &LegacyListAssessmentsResponse{
		Items:      items,
		Total:      list.Total,
		Page:       list.Page,
		PageSize:   list.PageSize,
		TotalPages: list.TotalPages,
	}
}

// ReportToLegacy projects outcome report onto the legacy REST shape.
func ReportToLegacy(report *AssessmentReportResponse) *LegacyAssessmentReportResponse {
	if report == nil {
		return nil
	}
	return &LegacyAssessmentReportResponse{
		AssessmentID: report.AssessmentID,
		ScaleCode:    legacyScaleCode(report.Model),
		ScaleName:    legacyScaleName(report.Model),
		TotalScore:   LegacyTotalScore(report.PrimaryScore),
		RiskLevel:    LegacyRiskLevel(report.Level),
		Conclusion:   report.Conclusion,
		Dimensions:   report.Dimensions,
		Suggestions:  report.Suggestions,
		CreatedAt:    report.CreatedAt,
	}
}
