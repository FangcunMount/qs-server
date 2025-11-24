package staff

// Staff 后台工作人员聚合根
// 设计说明：
// 1. Staff 是 IAM.User 在本 BC 的业务视图投影，不是完整的用户实体
// 2. 持久化的核心目的：
//   - 存储业务角色（roles）：这是本 BC 的领域概念，IAM 不管
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
type Staff struct {
	id       ID     // 内部员工ID（主键）
	orgID    int64  // 所属机构（多租户隔离）
	userID   int64  // 用户ID（外键，必须绑定）
	roles    []Role // 业务角色列表（核心业务数据）
	name     string // 姓名（缓存字段）
	email    string // 邮箱（缓存字段）
	phone    string // 手机号（缓存字段）
	isActive bool   // 在本系统内的激活状态
}

// NewStaff 创建新的员工
func NewStaff(orgID int64, userID int64, name string) *Staff {
	return &Staff{
		orgID:    orgID,
		userID:   userID,
		name:     name,
		roles:    make([]Role, 0),
		isActive: true,
	}
}

// === Getters ===

// ID 获取员工ID
func (s *Staff) ID() ID {
	return s.id
}

// OrgID 获取机构ID
func (s *Staff) OrgID() int64 {
	return s.orgID
}

// UserID 获取用户ID
func (s *Staff) UserID() int64 {
	return s.userID
}

// Roles 获取角色列表
func (s *Staff) Roles() []Role {
	return s.roles
}

// Name 获取姓名
func (s *Staff) Name() string {
	return s.name
}

// Email 获取邮箱
func (s *Staff) Email() string {
	return s.email
}

// Phone 获取手机号
func (s *Staff) Phone() string {
	return s.phone
}

// IsActive 是否激活
func (s *Staff) IsActive() bool {
	return s.isActive
}

// === Setters（用于仓储层）===

// SetID 设置ID
func (s *Staff) SetID(id ID) {
	s.id = id
}

// === 核心行为（包内可见，通过领域服务使用）===

// assignRole 分配角色（包内方法，应通过 RoleManager 调用）
func (s *Staff) assignRole(role Role) {
	// 防重复
	for _, existing := range s.roles {
		if existing == role {
			return
		}
	}
	s.roles = append(s.roles, role)
}

// removeRole 移除角色（包内方法，应通过 RoleManager 调用）
func (s *Staff) removeRole(role Role) {
	for i, existing := range s.roles {
		if existing == role {
			s.roles = append(s.roles[:i], s.roles[i+1:]...)
			return
		}
	}
}

// HasRole 检查是否有某个角色
func (s *Staff) HasRole(role Role) bool {
	for _, r := range s.roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole 检查是否有任意一个角色
func (s *Staff) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if s.HasRole(role) {
			return true
		}
	}
	return false
}

// updateContactInfo 更新联系信息（包内方法，应通过 Editor 或 IAMSynchronizer 调用）
func (s *Staff) updateContactInfo(email, phone string) {
	if email != "" {
		s.email = email
	}
	if phone != "" {
		s.phone = phone
	}
}

// activate 激活（包内方法，应通过 Editor 调用）
func (s *Staff) activate() {
	s.isActive = true
}

// deactivate 停用（包内方法，应通过 Editor 调用）
func (s *Staff) deactivate() {
	s.isActive = false
}

// CanManageScales 是否可以管理量表
func (s *Staff) CanManageScales() bool {
	return s.HasRole(RoleScaleAdmin)
}

// CanEvaluate 是否可以评估
func (s *Staff) CanEvaluate() bool {
	return s.HasRole(RoleEvaluator)
}

// CanManageScreeningProject 是否可以管理筛查项目
func (s *Staff) CanManageScreeningProject() bool {
	return s.HasRole(RoleScreeningOwner)
}

// CanAuditReport 是否可以审核报告
func (s *Staff) CanAuditReport() bool {
	return s.HasRole(RoleReportAuditor)
}

// === 仓储层重建方法（用于从数据库加载）===

// RestoreFromRepository 从仓储恢复聚合根状态（用于仓储层重建对象）
// 这些方法绕过领域服务的验证，仅用于从持久化存储加载数据
func (s *Staff) RestoreFromRepository(
	roles []Role,
	email string,
	phone string,
	isActive bool,
) {
	s.roles = make([]Role, len(roles))
	copy(s.roles, roles)
	s.email = email
	s.phone = phone
	s.isActive = isActive
}
