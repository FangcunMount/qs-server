package staff

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 人事/行政部门 (HR/Admin) - 负责员工生命周期管理、资料维护
// 2. IT管理员/权限管理员 (IT Admin) - 负责角色分配、权限管理
// 3. 通用查询服务 (Query Service) - 为所有行为者提供只读查询

// StaffLifecycleService 员工生命周期服务
// 行为者：人事/行政部门 (HR/Admin)
// 职责：员工入职、离职、基本信息维护
// 变更来源：人事部门的业务需求变化
type StaffLifecycleService interface {
	// Register 注册新员工（入职）
	// 场景：新员工入职时，人事部门录入员工信息
	Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error)

	// EnsureByUser 确保员工存在（幂等）
	// 场景：员工首次登录系统时，自动创建员工记录
	EnsureByUser(ctx context.Context, orgID int64, userID int64, name string) (*StaffResult, error)

	// Delete 删除员工（离职）
	// 场景：员工离职时，人事部门删除员工记录
	Delete(ctx context.Context, staffID uint64) error

	// UpdateContactInfo 更新联系方式
	// 场景：员工联系方式变更时，人事部门更新
	UpdateContactInfo(ctx context.Context, dto UpdateStaffContactDTO) error

	// UpdateFromExternalSource 从外部源同步员工信息
	// 场景：从IAM系统或其他人力资源系统同步员工信息
	UpdateFromExternalSource(ctx context.Context, staffID uint64, name, email, phone string) error
}

// StaffAuthorizationService 员工权限管理服务
// 行为者：IT管理员/权限管理员 (IT Admin)
// 职责：角色分配、权限管理、账号启停用
// 变更来源：IT部门的权限管理需求变化
type StaffAuthorizationService interface {
	// AssignRole 分配角色
	// 场景：IT管理员为员工分配系统角色
	AssignRole(ctx context.Context, staffID uint64, role string) error

	// RemoveRole 移除角色
	// 场景：IT管理员移除员工的某个角色
	RemoveRole(ctx context.Context, staffID uint64, role string) error

	// Activate 激活员工账号
	// 场景：IT管理员启用员工系统账号
	Activate(ctx context.Context, staffID uint64) error

	// Deactivate 停用员工账号
	// 场景：IT管理员暂时停用员工系统账号
	Deactivate(ctx context.Context, staffID uint64) error
}

// StaffQueryService 员工查询服务（只读）
// 行为者：所有需要查询员工信息的用户
// 职责：提供员工信息查询能力
// 变更来源：查询需求变化（通常较少变更）
type StaffQueryService interface {
	// GetByID 根据ID查询员工
	GetByID(ctx context.Context, staffID uint64) (*StaffResult, error)

	// GetByUser 根据用户ID查询员工
	// 场景：员工登录系统时查询员工信息
	GetByUser(ctx context.Context, orgID int64, userID int64) (*StaffResult, error)

	// ListStaffs 列出员工
	ListStaffs(ctx context.Context, dto ListStaffDTO) (*StaffListResult, error)
}

// ============= DTOs =============

// RegisterStaffDTO 注册员工 DTO
type RegisterStaffDTO struct {
	OrgID  int64    // 机构ID
	UserID int64    // 用户ID
	Name   string   // 姓名
	Email  string   // 邮箱
	Phone  string   // 手机号
	Roles  []string // 角色列表
}

// UpdateStaffContactDTO 更新员工联系方式 DTO
type UpdateStaffContactDTO struct {
	StaffID uint64 // 员工ID
	Email   string // 邮箱
	Phone   string // 手机号
}

// ListStaffDTO 列出员工 DTO
type ListStaffDTO struct {
	OrgID  int64  // 机构ID
	Role   string // 角色过滤
	Offset int    // 偏移量
	Limit  int    // 限制数量
}

// StaffResult 员工结果 DTO
type StaffResult struct {
	ID       uint64   // 员工ID
	OrgID    int64    // 机构ID
	UserID   int64    // 用户ID
	Roles    []string // 角色列表
	Name     string   // 姓名
	Email    string   // 邮箱
	Phone    string   // 手机号
	IsActive bool     // 是否激活
}

// StaffListResult 员工列表结果 DTO
type StaffListResult struct {
	Items      []*StaffResult // 员工列表
	TotalCount int64          // 总数
	Offset     int            // 偏移量
	Limit      int            // 限制数量
}
