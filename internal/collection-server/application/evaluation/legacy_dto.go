package evaluation

// LegacyAssessmentSummaryResponse 量表导向的测评摘要（deprecated REST v1 投影）。
type LegacyAssessmentSummaryResponse struct {
	ID                   string  `json:"id"`
	QuestionnaireCode    string  `json:"questionnaire_code"`
	QuestionnaireVersion string  `json:"questionnaire_version"`
	AnswerSheetID        string  `json:"answer_sheet_id,omitempty"`
	ScaleCode            string  `json:"scale_code,omitempty"`
	ScaleName            string  `json:"scale_name,omitempty"`
	OriginType           string  `json:"origin_type"`
	Status               string  `json:"status"`
	TotalScore           float64 `json:"total_score,omitempty"`
	RiskLevel            string  `json:"risk_level,omitempty"`
	CreatedAt            string  `json:"created_at"`
	SubmittedAt          string  `json:"submitted_at,omitempty"`
	InterpretedAt        string  `json:"interpreted_at,omitempty"`
}

// LegacyAssessmentDetailResponse 量表导向的测评详情（deprecated REST v1 投影）。
type LegacyAssessmentDetailResponse struct {
	ID                   string  `json:"id"`
	OrgID                string  `json:"org_id"`
	TesteeID             string  `json:"testee_id"`
	QuestionnaireCode    string  `json:"questionnaire_code"`
	QuestionnaireVersion string  `json:"questionnaire_version"`
	AnswerSheetID        string  `json:"answer_sheet_id,omitempty"`
	ScaleCode            string  `json:"scale_code,omitempty"`
	ScaleName            string  `json:"scale_name,omitempty"`
	OriginType           string  `json:"origin_type"`
	OriginID             string  `json:"origin_id,omitempty"`
	Status               string  `json:"status"`
	TotalScore           float64 `json:"total_score,omitempty"`
	RiskLevel            string  `json:"risk_level,omitempty"`
	CreatedAt            string  `json:"created_at"`
	SubmittedAt          string  `json:"submitted_at,omitempty"`
	InterpretedAt        string  `json:"interpreted_at,omitempty"`
	FailedAt             string  `json:"failed_at,omitempty"`
	FailureReason        string  `json:"failure_reason,omitempty"`
}

// LegacyAssessmentReportResponse 量表导向的测评报告（deprecated REST v1 投影）。
type LegacyAssessmentReportResponse struct {
	AssessmentID string                       `json:"assessment_id"`
	ScaleCode    string                       `json:"scale_code"`
	ScaleName    string                       `json:"scale_name"`
	TotalScore   float64                      `json:"total_score"`
	RiskLevel    string                       `json:"risk_level"`
	Conclusion   string                       `json:"conclusion"`
	Dimensions   []DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []SuggestionResponse         `json:"suggestions"`
	CreatedAt    string                       `json:"created_at"`
}

// LegacyListAssessmentsResponse 量表导向的测评列表（deprecated REST v1 投影）。
type LegacyListAssessmentsResponse struct {
	Items      []LegacyAssessmentSummaryResponse `json:"items"`
	Total      int32                             `json:"total"`
	Page       int32                             `json:"page"`
	PageSize   int32                             `json:"page_size"`
	TotalPages int32                             `json:"total_pages"`
}
