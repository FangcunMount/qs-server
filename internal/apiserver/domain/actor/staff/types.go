package staff

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 员工ID类型
type ID = meta.ID

// NewID 创建员工ID
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// Role 员工角色类型
type Role string

const (
	// RoleScaleAdmin 量表管理员
	RoleScaleAdmin Role = "scale_admin"
	// RoleEvaluator 评估人员
	RoleEvaluator Role = "evaluator"
	// RoleScreeningOwner 筛查项目负责人
	RoleScreeningOwner Role = "screening_owner"
	// RoleReportAuditor 报告审核员
	RoleReportAuditor Role = "report_auditor"
)
