package operator

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 员工ID类型
type ID = meta.ID

// Role 员工角色类型
type Role string

// 旧的业务角色标识已迁移到统一的权限中心字符串格式。
// 本文件保留新的统一角色常量，旧常量已删除以避免混淆。

func (r Role) String() string {
	return string(r)
}

// 新增的 QS 角色标识（与权限中心保持一致）
const (
	// RoleAssessmentOperator 组织测评与过程处理，不包含专业结果访问。
	RoleAssessmentOperator Role = "qs:assessment_operator"
	// RoleResultReviewer 查看与分析授权范围内的专业结果。
	RoleResultReviewer Role = "qs:result_reviewer"
	// RoleQSAdmin 管理员：所有 QS 资源的所有操作
	RoleQSAdmin Role = "qs:admin"
	// RoleContentManager 内容管理员：问卷和量表的完整管理
	RoleContentManager Role = "qs:content_manager"
	// RoleEvaluatorQS 评估员：测评相关只读 + 重试
	RoleEvaluatorQS Role = "qs:evaluator"
	// RoleEvaluationPlanManager 测评计划管理员：测评计划的管理权限
	RoleEvaluationPlanManager Role = "qs:evaluation_plan_manager"
	// RoleOperator 普通员工：只能查看受试者
	RoleOperator Role = "qs:staff"
)
