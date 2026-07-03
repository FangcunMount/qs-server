package reportwait

import (
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
)

// ToPublicAssessmentStatus 将内部状态映射为 HTTP 对外契约（completed → interpreted）。
func ToPublicAssessmentStatus(resp *evaluationapp.AssessmentStatusResponse) *evaluationapp.AssessmentStatusResponse {
	return reportstatus.ToPublicAssessmentStatus(resp)
}
