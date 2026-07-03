package personalityassessment

import evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"

type (
	ModelIdentityResponse = evaluationapp.ModelIdentityResponse
	ScoreValueResponse    = evaluationapp.ScoreValueResponse
	ResultLevelResponse   = evaluationapp.ResultLevelResponse
	ModelExtraResponse    = evaluationapp.ModelExtraResponse
	ModelRarityResponse   = evaluationapp.ModelRarityResponse
)

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
	AssessmentID string                                     `json:"assessment_id"`
	Model        ModelIdentityResponse                      `json:"model"`
	PrimaryScore *ScoreValueResponse                        `json:"primary_score,omitempty"`
	Level        *ResultLevelResponse                       `json:"level,omitempty"`
	Conclusion   string                                     `json:"conclusion"`
	Dimensions   []evaluationapp.DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []evaluationapp.SuggestionResponse           `json:"suggestions"`
	ModelExtra   *ModelExtraResponse                        `json:"model_extra,omitempty"`
	CreatedAt    string                                     `json:"created_at"`
}

type AssessmentStatusResponse struct {
	Status          string                 `json:"status"`
	Stage           string                 `json:"stage,omitempty"`
	Message         string                 `json:"message,omitempty"`
	Reason          string                 `json:"reason,omitempty"`
	NextPollAfterMs int                    `json:"next_poll_after_ms,omitempty"`
	Model           *ModelIdentityResponse `json:"model,omitempty"`
	Level           *ResultLevelResponse   `json:"level,omitempty"`
	UpdatedAt       int64                  `json:"updated_at"`
}
