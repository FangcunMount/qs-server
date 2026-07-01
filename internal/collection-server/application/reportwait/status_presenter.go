package reportwait

import (
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

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
