package staff_management

import "context"

// ============= 应用服务接口（Driving Ports）=============

// StaffApplicationService 员工应用服务
// 操作者：管理员（Admin）
// 场景：后台管理系统创建/删除员工
type StaffApplicationService interface {
	// Register 注册新员工
	Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error)

	// EnsureByIAMUser 确保员工存在（幂等）
	// 场景：员工首次登录后台系统
	EnsureByIAMUser(ctx context.Context, orgID int64, iamUserID int64, name string) (*StaffResult, error)

	// Delete 删除员工
	Delete(ctx context.Context, staffID uint64) error
}

// StaffProfileApplicationService 员工资料应用服务
// 操作者：管理员（Admin）
// 场景：更新员工基本信息
type StaffProfileApplicationService interface {
	// UpdateContactInfo 更新联系方式
	UpdateContactInfo(ctx context.Context, dto UpdateStaffContactDTO) error

	// SyncFromIAM 从IAM同步员工信息
	// 场景：定时任务或手动触发
	SyncFromIAM(ctx context.Context, staffID uint64, name, email, phone string) error
}

// StaffRoleApplicationService 员工角色应用服务
// 操作者：管理员（Admin）
// 场景：管理员工角色和权限
type StaffRoleApplicationService interface {
	// AssignRole 分配角色
	AssignRole(ctx context.Context, staffID uint64, role string) error

	// RemoveRole 移除角色
	RemoveRole(ctx context.Context, staffID uint64, role string) error

	// Activate 激活员工
	Activate(ctx context.Context, staffID uint64) error

	// Deactivate 停用员工
	Deactivate(ctx context.Context, staffID uint64) error
}

// StaffQueryApplicationService 员工查询应用服务（只读）
// 操作者：管理员（Admin）
// 场景：查询员工信息
type StaffQueryApplicationService interface {
	// GetByID 根据ID查询员工
	GetByID(ctx context.Context, staffID uint64) (*StaffResult, error)

	// GetByIAMUser 根据IAM用户ID查询员工
	// 场景：员工登录后台系统时
	GetByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*StaffResult, error)

	// ListStaffs 列出员工
	ListStaffs(ctx context.Context, dto ListStaffDTO) (*StaffListResult, error)
}

// ============= DTOs =============

// RegisterStaffDTO 注册员工 DTO
type RegisterStaffDTO struct {
	OrgID     int64    // 机构ID
	IAMUserID int64    // IAM用户ID
	Name      string   // 姓名
	Email     string   // 邮箱
	Phone     string   // 手机号
	Roles     []string // 角色列表
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
	ID        uint64   // 员工ID
	OrgID     int64    // 机构ID
	IAMUserID int64    // IAM用户ID
	Roles     []string // 角色列表
	Name      string   // 姓名
	Email     string   // 邮箱
	Phone     string   // 手机号
	IsActive  bool     // 是否激活
}

// StaffListResult 员工列表结果 DTO
type StaffListResult struct {
	Items      []*StaffResult // 员工列表
	TotalCount int64          // 总数
	Offset     int            // 偏移量
	Limit      int            // 限制数量
}
