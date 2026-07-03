package assessment

import "time"

// ModelIdentityResult is the published-model reference on outcome read APIs.
type ModelIdentityResult struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ScoreValueResult is the primary score projection on outcome read APIs.
type ScoreValueResult struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResult is the outcome level projection on outcome read APIs.
type ResultLevelResult struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentOutcomeResult exposes assessment facts with outcome summary.
type AssessmentOutcomeResult struct {
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

// AssessmentOutcomeListResult is a paginated outcome assessment list.
type AssessmentOutcomeListResult struct {
	Items      []*AssessmentOutcomeResult `json:"items"`
	Total      int                        `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}

// ReportOutcomeResult exposes report facts with outcome summary.
type ReportOutcomeResult struct {
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

// ReportOutcomeListResult is a paginated outcome report list.
type ReportOutcomeListResult struct {
	Items      []*ReportOutcomeResult `json:"items"`
	Total      int                    `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int                    `json:"total_pages"`
}

// Deprecated: use AssessmentOutcomeResult.
type AssessmentV2Result = AssessmentOutcomeResult

// Deprecated: use AssessmentOutcomeListResult.
type AssessmentV2ListResult = AssessmentOutcomeListResult

// Deprecated: use ReportOutcomeResult.
type ReportV2Result = ReportOutcomeResult

// Deprecated: use ReportOutcomeListResult.
type ReportV2ListResult = ReportOutcomeListResult
