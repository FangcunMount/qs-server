package operator

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 人事/行政部门 (HR/Admin) - 负责操作者生命周期管理、资料维护
// 2. IT管理员/权限管理员 (IT Admin) - 负责角色分配、权限管理
// 3. 通用查询服务 (Query Service) - 为所有行为者提供只读查询

// OperatorLifecycleService 操作者生命周期服务
// 行为者：人事/行政部门 (HR/Admin)
// 职责：后台操作者入职、离职、基本信息维护
// 变更来源：人事部门的业务需求变化
type OperatorLifecycleService interface {
	// Register 注册新操作者（入职）
	Register(ctx context.Context, dto RegisterOperatorDTO) (*OperatorResult, error)

	// EnsureByUser 确保操作者存在（幂等）
	EnsureByUser(ctx context.Context, orgID int64, userID int64, name string) (*OperatorResult, error)

	// Delete 删除操作者（离职）
	Delete(ctx context.Context, operatorID uint64) error

	// UpdateProfile 更新本地员工投影资料
	UpdateProfile(ctx context.Context, dto UpdateOperatorProfileDTO) (*OperatorResult, error)

	// UpdateContactInfo 更新联系方式
	// 场景：成员联系方式变更时，人事部门更新
	UpdateContactInfo(ctx context.Context, dto UpdateOperatorContactDTO) error

	// UpdateFromExternalSource 从外部源同步操作者信息
	UpdateFromExternalSource(ctx context.Context, operatorID uint64, name, email, phone string) error
}

// OperatorAuthorizationService 操作者权限管理服务
// 行为者：IT管理员/权限管理员 (IT Admin)
// 职责：角色分配、权限管理、账号启停用
// 变更来源：IT部门的权限管理需求变化
type OperatorAuthorizationService interface {
	// AssignRole 分配角色
	AssignRole(ctx context.Context, operatorID uint64, role string) error

	// RemoveRole 移除角色
	RemoveRole(ctx context.Context, operatorID uint64, role string) error

	// Activate 激活操作者账号
	Activate(ctx context.Context, operatorID uint64) error

	// Deactivate 停用操作者账号
	Deactivate(ctx context.Context, operatorID uint64) error
}

// OperatorQueryService 操作者查询服务（只读）
// 行为者：所有需要查询后台操作者信息的用户
// 职责：提供操作者信息查询能力
// 变更来源：查询需求变化（通常较少变更）
type OperatorQueryService interface {
	// GetByID 根据ID查询操作者
	GetByID(ctx context.Context, operatorID uint64) (*OperatorResult, error)

	// GetByUser 根据用户ID查询操作者
	GetByUser(ctx context.Context, orgID int64, userID int64) (*OperatorResult, error)

	// ListOperators 列出机构内操作者
	ListOperators(ctx context.Context, dto ListOperatorDTO) (*OperatorListResult, error)
}

// ============= DTOs =============

// RegisterOperatorDTO 注册操作者 DTO
type RegisterOperatorDTO struct {
	OrgID    int64    // 机构ID
	UserID   int64    // 用户ID
	Name     string   // 姓名
	Email    string   // 邮箱
	Phone    string   // 手机号
	Roles    []string // 角色列表
	IsActive bool     // 是否激活
}

// UpdateOperatorProfileDTO 更新员工资料 DTO
type UpdateOperatorProfileDTO struct {
	OperatorID uint64  // 操作者ID
	Name       *string // 姓名
	Email      *string // 邮箱
	Phone      *string // 手机号
}

// UpdateOperatorContactDTO 更新操作者联系方式 DTO
type UpdateOperatorContactDTO struct {
	OperatorID uint64 // 操作者ID
	Email      string // 邮箱
	Phone      string // 手机号
}

// ListOperatorDTO 列出操作者 DTO
type ListOperatorDTO struct {
	OrgID  int64  // 机构ID
	Role   string // 角色过滤
	Offset int    // 偏移量
	Limit  int    // 限制数量
}

// OperatorResult 操作者结果 DTO
type OperatorResult struct {
	ID       uint64   // 操作者ID
	OrgID    int64    // 机构ID
	UserID   int64    // 用户ID
	Roles    []string // 角色列表
	Name     string   // 姓名
	Email    string   // 邮箱
	Phone    string   // 手机号
	IsActive bool     // 是否激活
}

// OperatorListResult 操作者列表结果 DTO
type OperatorListResult struct {
	Items      []*OperatorResult // 员工列表
	TotalCount int64             // 总数
	Offset     int               // 偏移量
	Limit      int               // 限制数量
}
