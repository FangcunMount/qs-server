package evaluation

// AssessmentSummaryResponse 测评摘要响应
type AssessmentSummaryResponse struct {
	ID                   string  `json:"id"`
	QuestionnaireCode    string  `json:"questionnaire_code"`
	QuestionnaireVersion string  `json:"questionnaire_version"`
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

// AssessmentDetailResponse 测评详情响应
type AssessmentDetailResponse struct {
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

// FactorScoreResponse 因子得分响应
type FactorScoreResponse struct {
	FactorCode   string  `json:"factor_code"`
	FactorName   string  `json:"factor_name"`
	RawScore     float64 `json:"raw_score"`
	RiskLevel    string  `json:"risk_level,omitempty"`
	Conclusion   string  `json:"conclusion,omitempty"`
	Suggestion   string  `json:"suggestion,omitempty"`
	IsTotalScore bool    `json:"is_total_score"`
}

// DimensionInterpretResponse 维度解读响应
type DimensionInterpretResponse struct {
	FactorCode  string  `json:"factor_code"`
	FactorName  string  `json:"factor_name"`
	RawScore    float64 `json:"raw_score"`
	RiskLevel   string  `json:"risk_level"`
	Description string  `json:"description"`
}

// AssessmentReportResponse 测评报告响应
type AssessmentReportResponse struct {
	AssessmentID string                       `json:"assessment_id"`
	ScaleCode    string                       `json:"scale_code"`
	ScaleName    string                       `json:"scale_name"`
	TotalScore   float64                      `json:"total_score"`
	RiskLevel    string                       `json:"risk_level"`
	Conclusion   string                       `json:"conclusion"`
	Dimensions   []DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []string                     `json:"suggestions"`
	CreatedAt    string                       `json:"created_at"`
}

// ListAssessmentsRequest 测评列表请求
type ListAssessmentsRequest struct {
	Status   string `form:"status"`
	Page     int32  `form:"page"`
	PageSize int32  `form:"page_size"`
}

// ListAssessmentsResponse 测评列表响应
type ListAssessmentsResponse struct {
	Items      []AssessmentSummaryResponse `json:"items"`
	Total      int32                       `json:"total"`
	Page       int32                       `json:"page"`
	PageSize   int32                       `json:"page_size"`
	TotalPages int32                       `json:"total_pages"`
}

// TrendPointResponse 趋势数据点响应
type TrendPointResponse struct {
	AssessmentID string  `json:"assessment_id"`
	Score        float64 `json:"score"`
	RiskLevel    string  `json:"risk_level"`
	CreatedAt    string  `json:"created_at"`
}

// GetFactorTrendRequest 获取因子趋势请求
type GetFactorTrendRequest struct {
	FactorCode string `form:"factor_code" binding:"required"`
	Limit      int32  `form:"limit"`
}
