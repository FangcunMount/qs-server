package reportstatus

import (
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

// View 是 HTTP report-status 与 WS events 共用的对外状态视图。
type View struct {
	Status          string   `json:"status"`
	Stage           string   `json:"stage,omitempty"`
	Message         string   `json:"message,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	NextPollAfterMs int      `json:"next_poll_after_ms,omitempty"`
	TotalScore      *float64 `json:"total_score,omitempty"`
	RiskLevel       *string  `json:"risk_level,omitempty"`
	UpdatedAt       int64    `json:"updated_at"`
}

// ToPublicAssessmentStatus 将内部状态映射为 HTTP 对外契约（completed → interpreted）。
func ToPublicAssessmentStatus(resp *evaluationapp.AssessmentStatusResponse) *evaluationapp.AssessmentStatusResponse {
	if resp == nil {
		return nil
	}
	out := *resp
	if out.Status == "completed" {
		out.Status = "interpreted"
	}
	return &out
}

// MedicalView 将 medical 状态映射为 HTTP/WS 共用视图。
func MedicalView(status *evaluationapp.AssessmentStatusResponse) *View {
	public := ToPublicAssessmentStatus(status)
	if public == nil {
		return nil
	}
	return &View{
		Status:          public.Status,
		Stage:           public.Stage,
		Message:         public.Message,
		Reason:          public.Reason,
		NextPollAfterMs: public.NextPollAfterMs,
		TotalScore:      public.TotalScore,
		RiskLevel:       public.RiskLevel,
		UpdatedAt:       public.UpdatedAt,
	}
}

// IsTerminalStatus 判断对外状态是否终态。
func IsTerminalStatus(status string) bool {
	return status == "interpreted" || status == "failed" || status == "completed"
}
