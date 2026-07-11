package assessment

import "time"

// ModelIdentityResult 是published-模型引用 on 结果 read APIs。
type ModelIdentityResult struct {
	Kind            string `json:"kind"`
	SubKind         string `json:"sub_kind,omitempty"`
	Algorithm       string `json:"algorithm,omitempty"`
	Code            string `json:"code"`
	Version         string `json:"version,omitempty"`
	Title           string `json:"title,omitempty"`
	ProductChannel  string `json:"product_channel,omitempty"`
	AlgorithmFamily string `json:"algorithm_family,omitempty"`
}

// ScoreValueResult 是主 score 投影 on 结果 read APIs。
type ScoreValueResult struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResult 是结果 等级 投影 on 结果 read APIs。
type ResultLevelResult struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentOutcomeResult 暴露assessment 事实 使用 结果 summary。
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
	FailedAt             *time.Time          `json:"failed_at,omitempty"`
	FailureReason        *string             `json:"failure_reason,omitempty"`
}

// AssessmentOutcomeListResult 是paginated 结果 assessment list。
type AssessmentOutcomeListResult struct {
	Items      []*AssessmentOutcomeResult `json:"items"`
	Total      int                        `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}
