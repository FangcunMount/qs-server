package operator

import "time"

// Operator 后台工作人员聚合根
// 设计说明：
// 1. Operator 是 IAM.User 在本 BC 的业务视图投影，不是完整的用户实体
// 2. 持久化的核心目的：
//   - 存储业务角色（roles）：GetAuthorizationSnapshot.roles 的本地只读投影
//   - 多租户隔离：同一 IAM.User 在不同机构可能有不同角色
//   - 审计追溯：操作记录用 ID 比 IAMUserID 更有业务语义
//   - 性能优化：缓存常用字段（name），减少 RPC 调用
//
// 3. 不存储 IAM 的认证信息（密码、token 等），通过 iamUserID 关联
//
// 4. 设计原则：
//   - 以行为为中心，通过领域服务管理复杂逻辑
//   - 不过度暴露内部状态，保持封装性
//   - 审计字段由基础设施层（PO）处理
type Operator struct {
	id                     ID     // 内部员工ID（主键）
	orgID                  int64  // 所属机构（多租户隔离）
	userID                 int64  // 用户ID（外键，必须绑定）
	roles                  []Role // IAM 直接角色的只读投影；列名为兼容保留
	effectiveRoles         []Role
	authzPolicyVersion     int64
	authzProjectedAt       *time.Time
	authzProjectionPending bool
	name                   string // 姓名（缓存字段）
	email                  string // 邮箱（缓存字段）
	phone                  string // 手机号（缓存字段）
	isActive               bool   // 在本系统内的激活状态
}

// NewOperator 创建新的后台操作者
func NewOperator(orgID int64, userID int64, name string) *Operator {
	return &Operator{
		orgID:          orgID,
		userID:         userID,
		name:           name,
		roles:          make([]Role, 0),
		effectiveRoles: make([]Role, 0),
		isActive:       true,
	}
}

// === Getter 访问方法 ===

// ID 获取员工ID
func (s *Operator) ID() ID {
	return s.id
}

// OrgID 获取机构ID
func (s *Operator) OrgID() int64 {
	return s.orgID
}

// UserID 获取用户ID
func (s *Operator) UserID() int64 {
	return s.userID
}

// Roles 获取角色列表
func (s *Operator) Roles() []Role {
	roles := make([]Role, len(s.roles))
	copy(roles, s.roles)
	return roles
}

func (s *Operator) EffectiveRoles() []Role {
	roles := make([]Role, len(s.effectiveRoles))
	copy(roles, s.effectiveRoles)
	return roles
}

func (s *Operator) AuthzPolicyVersion() int64 { return s.authzPolicyVersion }

func (s *Operator) AuthzProjectedAt() *time.Time {
	if s.authzProjectedAt == nil {
		return nil
	}
	value := *s.authzProjectedAt
	return &value
}

func (s *Operator) AuthzProjectionPending() bool { return s.authzProjectionPending }

func (s *Operator) MarkAuthzProjectionPending() {
	s.authzProjectionPending = true
}

// Name 获取姓名
func (s *Operator) Name() string {
	return s.name
}

// Email 获取邮箱
func (s *Operator) Email() string {
	return s.email
}

// Phone 获取手机号
func (s *Operator) Phone() string {
	return s.phone
}

// IsActive 是否激活
func (s *Operator) IsActive() bool {
	return s.isActive
}

// === Setters（用于仓储层）===

// SetID 设置ID
func (s *Operator) SetID(id ID) {
	s.id = id
}

// HasRole 检查是否有某个角色
func (s *Operator) HasRole(role Role) bool {
	for _, r := range s.roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole 检查是否有任意一个角色
func (s *Operator) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if s.HasRole(role) {
			return true
		}
	}
	return false
}

// activate 激活（包内方法，应通过 Editor 调用）
func (s *Operator) activate() {
	s.isActive = true
}

// deactivate 停用（包内方法，应通过 Editor 调用）
func (s *Operator) deactivate() {
	s.isActive = false
}

// CanEvaluate 是否可以评估
func (s *Operator) CanEvaluate() bool {
	// QS 评估员或管理员可以评估
	return s.HasAnyRole(RoleEvaluatorQS, RoleQSAdmin)
}

// CanAuditReport 是否可以审核报告
func (s *Operator) CanAuditReport() bool {
	// 目前仅管理员可以审核报告
	return s.HasRole(RoleQSAdmin)
}

// CanManageEvaluationPlans 是否可以管理测评计划
func (s *Operator) CanManageEvaluationPlans() bool {
	// 管理员或测评计划管理员可以管理测评计划
	return s.HasAnyRole(RoleQSAdmin, RoleEvaluationPlanManager)
}

// === 仓储层重建方法（用于从数据库加载）===

// ReplaceRolesProjection replaces the non-authoritative IAM role projection.
func (s *Operator) ReplaceRolesProjection(directRoles, effectiveRoles []Role, policyVersion int64, projectedAt *time.Time, pending bool) {
	s.roles = append([]Role(nil), directRoles...)
	s.effectiveRoles = append([]Role(nil), effectiveRoles...)
	s.authzPolicyVersion = policyVersion
	s.authzProjectionPending = pending
	if projectedAt == nil {
		s.authzProjectedAt = nil
	} else {
		value := *projectedAt
		s.authzProjectedAt = &value
	}
}

// RestoreFromRepository 从仓储恢复聚合根状态（用于仓储层重建对象）
// 这些方法绕过领域服务的验证，仅用于从持久化存储加载数据
func (s *Operator) RestoreFromRepository(
	roles []Role,
	effectiveRoles []Role,
	authzPolicyVersion int64,
	authzProjectedAt *time.Time,
	authzProjectionPending bool,
	email string,
	phone string,
	isActive bool,
) {
	s.roles = append([]Role(nil), roles...)
	s.effectiveRoles = append([]Role(nil), effectiveRoles...)
	s.authzPolicyVersion = authzPolicyVersion
	s.authzProjectionPending = authzProjectionPending
	if authzProjectedAt != nil {
		value := *authzProjectedAt
		s.authzProjectedAt = &value
	}
	s.email = email
	s.phone = phone
	s.isActive = isActive
}
