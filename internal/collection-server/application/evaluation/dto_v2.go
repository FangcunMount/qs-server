package evaluation

// ModelIdentityResponse is the v2 published-model reference on collection REST APIs.
type ModelIdentityResponse struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ScoreValueResponse is the v2 primary score projection.
type ScoreValueResponse struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResponse is the v2 outcome level projection.
type ResultLevelResponse struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentDetailV2Response exposes assessment facts with v2 outcome summary.
type AssessmentDetailV2Response struct {
	ID                   string                `json:"id"`
	OrgID                string                `json:"org_id"`
	TesteeID             string                `json:"testee_id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	AnswerSheetID        string                `json:"answer_sheet_id,omitempty"`
	Model                ModelIdentityResponse `json:"model"`
	PrimaryScore         *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level                *ResultLevelResponse  `json:"level,omitempty"`
	OriginType           string                `json:"origin_type"`
	OriginID             string                `json:"origin_id,omitempty"`
	Status               string                `json:"status"`
	CreatedAt            string                `json:"created_at"`
	SubmittedAt          string                `json:"submitted_at,omitempty"`
	InterpretedAt        string                `json:"interpreted_at,omitempty"`
	FailedAt             string                `json:"failed_at,omitempty"`
	FailureReason        string                `json:"failure_reason,omitempty"`
}

// AssessmentSummaryV2Response is the list item projection for v2 APIs.
type AssessmentSummaryV2Response struct {
	ID                   string                `json:"id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	AnswerSheetID        string                `json:"answer_sheet_id,omitempty"`
	Model                ModelIdentityResponse `json:"model"`
	PrimaryScore         *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level                *ResultLevelResponse  `json:"level,omitempty"`
	OriginType           string                `json:"origin_type"`
	Status               string                `json:"status"`
	CreatedAt            string                `json:"created_at"`
	SubmittedAt          string                `json:"submitted_at,omitempty"`
	InterpretedAt        string                `json:"interpreted_at,omitempty"`
}

// ListAssessmentsV2Response is a paginated v2 assessment list.
type ListAssessmentsV2Response struct {
	Items      []AssessmentSummaryV2Response `json:"items"`
	Total      int32                         `json:"total"`
	Page       int32                         `json:"page"`
	PageSize   int32                         `json:"page_size"`
	TotalPages int32                         `json:"total_pages"`
}

// AssessmentReportV2Response exposes report facts with v2 outcome summary.
type AssessmentReportV2Response struct {
	AssessmentID string                       `json:"assessment_id"`
	Model        ModelIdentityResponse        `json:"model"`
	PrimaryScore *ScoreValueResponse          `json:"primary_score,omitempty"`
	Level        *ResultLevelResponse         `json:"level,omitempty"`
	Conclusion   string                       `json:"conclusion"`
	Dimensions   []DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []SuggestionResponse         `json:"suggestions"`
	CreatedAt    string                       `json:"created_at"`
}
