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

// 旧的业务角色标识已迁移到统一的权限中心字符串格式。
// 本文件保留新的统一角色常量，旧常量已删除以避免混淆。

func (r Role) String() string {
	return string(r)
}

// 新增的 QS 角色标识（与权限中心保持一致）
const (
	// RoleQSAdmin 管理员：所有 QS 资源的所有操作
	RoleQSAdmin Role = "qs:admin"
	// RoleContentManager 内容管理员：问卷和量表的完整管理
	RoleContentManager Role = "qs:content_manager"
	// RoleEvaluatorQS 评估员：测评相关只读 + 重试
	RoleEvaluatorQS Role = "qs:evaluator"
	// RoleEvaluationPlanManager 测评计划管理员：测评计划的管理权限
	RoleEvaluationPlanManager Role = "qs:evaluation_plan_manager"
	// RoleScreeningPlanManager 筛查计划管理员：筛查计划的管理权限
	RoleScreeningPlanManager Role = "qs:screening_plan_manager"
	// RoleStaff 普通员工：只能查看受试者
	RoleStaff Role = "qs:staff"
)
