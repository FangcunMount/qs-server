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

// StatusFields 是非 medical 状态源映射为 View 时使用的字段载体。
type StatusFields struct {
	Status          string
	Stage           string
	Message         string
	Reason          string
	NextPollAfterMs int
	TotalScore      *float64
	RiskLevel       *string
	UpdatedAt       int64
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
	return ViewFromFields(StatusFields{
		Status:          public.Status,
		Stage:           public.Stage,
		Message:         public.Message,
		Reason:          public.Reason,
		NextPollAfterMs: public.NextPollAfterMs,
		TotalScore:      public.TotalScore,
		RiskLevel:       public.RiskLevel,
		UpdatedAt:       public.UpdatedAt,
	})
}

// PersonalityView 将 personality 状态字段映射为 HTTP/WS 共用视图。
func PersonalityView(fields StatusFields) *View {
	return ViewFromFields(fields)
}

// ViewFromFields 统一构造 HTTP report-status 与 WS events 的对外状态视图。
func ViewFromFields(fields StatusFields) *View {
	return &View{
		Status:          fields.Status,
		Stage:           fields.Stage,
		Message:         fields.Message,
		Reason:          fields.Reason,
		NextPollAfterMs: fields.NextPollAfterMs,
		TotalScore:      fields.TotalScore,
		RiskLevel:       fields.RiskLevel,
		UpdatedAt:       fields.UpdatedAt,
	}
}

// IsTerminalStatus 判断对外状态是否终态。
func IsTerminalStatus(status string) bool {
	return status == "interpreted" || status == "failed" || status == "completed" || status == "temporarily_unavailable"
}
