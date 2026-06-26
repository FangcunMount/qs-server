package assessment

import "time"

// ModelIdentityResult is the v2 published-model reference on read APIs.
type ModelIdentityResult struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ScoreValueResult is the v2 primary score projection.
type ScoreValueResult struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResult is the v2 outcome level projection.
type ResultLevelResult struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentV2Result exposes assessment facts with v2 outcome summary.
type AssessmentV2Result struct {
	ID                   uint64              `json:"id"`
	OrgID                uint64              `json:"org_id"`
	TesteeID             uint64              `json:"testee_id"`
	QuestionnaireCode    string              `json:"questionnaire_code"`
	QuestionnaireVersion string              `json:"questionnaire_version"`
	AnswerSheetID        uint64              `json:"answer_sheet_id"`
	Model                ModelIdentityResult `json:"model"`
	PrimaryScore         *ScoreValueResult   `json:"primary_score,omitempty"`
	Level                *ResultLevelResult  `json:"level,omitempty"`
	OriginType           string              `json:"origin_type"`
	OriginID             *string             `json:"origin_id,omitempty"`
	Status               string              `json:"status"`
	SubmittedAt          *time.Time          `json:"submitted_at,omitempty"`
	InterpretedAt        *time.Time          `json:"interpreted_at,omitempty"`
	FailedAt             *time.Time          `json:"failed_at,omitempty"`
	FailureReason        *string             `json:"failure_reason,omitempty"`
}

// AssessmentV2ListResult is a paginated v2 assessment list.
type AssessmentV2ListResult struct {
	Items      []*AssessmentV2Result `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// ReportV2Result exposes report facts with v2 outcome summary.
type ReportV2Result struct {
	AssessmentID uint64              `json:"assessment_id"`
	Model        ModelIdentityResult `json:"model"`
	PrimaryScore *ScoreValueResult   `json:"primary_score,omitempty"`
	Level        *ResultLevelResult  `json:"level,omitempty"`
	Conclusion   string              `json:"conclusion"`
	Dimensions   []DimensionResult   `json:"dimensions"`
	Suggestions  []SuggestionDTO     `json:"suggestions"`
	ModelExtra   *ModelExtraResult   `json:"model_extra,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
}

// ReportV2ListResult is a paginated v2 report list.
type ReportV2ListResult struct {
	Items      []*ReportV2Result `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}
