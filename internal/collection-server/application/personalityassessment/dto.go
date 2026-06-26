package personalityassessment

import (
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

type ModelIdentityResponse struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

type ScoreValueResponse struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

type ResultLevelResponse struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

type ModelExtraResponse struct {
	Kind           string               `json:"kind,omitempty"`
	TypeCode       string               `json:"type_code,omitempty"`
	TypeName       string               `json:"type_name,omitempty"`
	OneLiner       string               `json:"one_liner,omitempty"`
	ImageURL       string               `json:"image_url,omitempty"`
	MatchPercent   float64              `json:"match_percent,omitempty"`
	IsSpecial      bool                 `json:"is_special,omitempty"`
	SpecialTrigger string               `json:"special_trigger,omitempty"`
	Commentary     string               `json:"commentary,omitempty"`
	Rarity         *ModelRarityResponse `json:"rarity,omitempty"`
}

type ModelRarityResponse struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int32   `json:"one_in_x,omitempty"`
}

type AssessmentDetailResponse struct {
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
	SubmittedAt          string                `json:"submitted_at,omitempty"`
	InterpretedAt        string                `json:"interpreted_at,omitempty"`
	FailedAt             string                `json:"failed_at,omitempty"`
	FailureReason        string                `json:"failure_reason,omitempty"`
}

type AssessmentSummaryResponse struct {
	ID                   string                `json:"id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	AnswerSheetID        string                `json:"answer_sheet_id,omitempty"`
	Model                ModelIdentityResponse `json:"model"`
	PrimaryScore         *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level                *ResultLevelResponse  `json:"level,omitempty"`
	OriginType           string                `json:"origin_type"`
	Status               string                `json:"status"`
	SubmittedAt          string                `json:"submitted_at,omitempty"`
	InterpretedAt        string                `json:"interpreted_at,omitempty"`
}

type ListAssessmentsRequest struct {
	Status    string `form:"status"`
	Algorithm string `form:"algorithm"`
	Page      int32  `form:"page"`
	PageSize  int32  `form:"page_size"`
}

type ListAssessmentsResponse struct {
	Items      []AssessmentSummaryResponse `json:"items"`
	Total      int32                       `json:"total"`
	Page       int32                       `json:"page"`
	PageSize   int32                       `json:"page_size"`
	TotalPages int32                       `json:"total_pages"`
}

type AssessmentReportResponse struct {
	AssessmentID string                           `json:"assessment_id"`
	Model        ModelIdentityResponse            `json:"model"`
	PrimaryScore *ScoreValueResponse              `json:"primary_score,omitempty"`
	Level        *ResultLevelResponse             `json:"level,omitempty"`
	Conclusion   string                           `json:"conclusion"`
	Dimensions   []evaluationapp.DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []evaluationapp.SuggestionResponse         `json:"suggestions"`
	ModelExtra   *ModelExtraResponse              `json:"model_extra,omitempty"`
	CreatedAt    string                           `json:"created_at"`
}

type AssessmentStatusResponse struct {
	Status          string                `json:"status"`
	Stage           string                `json:"stage,omitempty"`
	Message         string                `json:"message,omitempty"`
	Reason          string                `json:"reason,omitempty"`
	NextPollAfterMs int                   `json:"next_poll_after_ms,omitempty"`
	Model           *ModelIdentityResponse `json:"model,omitempty"`
	Level           *ResultLevelResponse  `json:"level,omitempty"`
	UpdatedAt       int64                 `json:"updated_at"`
}
