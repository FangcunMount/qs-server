package typologyassessment

import evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"

type (
	ModelIdentityResponse     = evaluationapp.ModelIdentityResponse
	ScoreValueResponse        = evaluationapp.ScoreValueResponse
	ResultLevelResponse       = evaluationapp.ResultLevelResponse
	ModelExtraResponse        = evaluationapp.ModelExtraResponse
	ModelRarityResponse       = evaluationapp.ModelRarityResponse
	AssessmentDetailResponse  = evaluationapp.AssessmentDetailResponse
	AssessmentSummaryResponse = evaluationapp.AssessmentSummaryResponse
	ListAssessmentsResponse   = evaluationapp.ListAssessmentsResponse
	AssessmentReportResponse  = evaluationapp.AssessmentReportResponse
)

type ListAssessmentsRequest struct {
	Status   string `form:"status"`
	Page     int32  `form:"page"`
	PageSize int32  `form:"page_size"`
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
