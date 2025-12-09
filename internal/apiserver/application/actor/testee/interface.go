package testee

import (
	"context"
	"time"
)

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP) + 分离关注点
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. C端用户(患者/家长) - 自助注册、查看自己的档案
// 2. B端员工(Staff) - 管理受试者档案、标签、重点关注、批量操作
// 3. 通用查询服务 (Query Service) - 为所有行为者提供只读查询

// TesteeRegistrationService 受试者注册服务
// 行为者：C端用户(患者/家长)
// 职责：受试者自助注册、档案创建
// 变更来源：C端用户注册流程的需求变化
type TesteeRegistrationService interface {
	// Register 注册受试者
	// 场景：患者/家长首次使用测评系统，填写基本信息注册
	Register(ctx context.Context, dto RegisterTesteeDTO) (*TesteeResult, error)

	// EnsureByProfile 确保受试者存在（幂等）
	// 场景：测评计划创建、答卷提交前，自动创建受试者档案
	EnsureByProfile(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error)

	// GetMyProfile 获取我的受试者档案
	// 场景：C端用户查看自己的测评档案
	GetMyProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error)
}

// TesteeManagementService 受试者档案管理服务
// 行为者：B端员工(Staff)
// 职责：管理受试者档案、业务标签、重点关注
// 变更来源：B端管理后台的业务需求变化
type TesteeManagementService interface {
	// UpdateBasicInfo 更新基本信息
	// 场景：后台员工修正受试者的姓名、性别、生日等基本信息
	UpdateBasicInfo(ctx context.Context, dto UpdateTesteeProfileDTO) error

	// BindProfile 绑定用户档案
	// 场景：将临时创建的受试者绑定到正式的用户档案
	BindProfile(ctx context.Context, testeeID uint64, profileID uint64) error

	// AddTag 添加业务标签
	// 场景：后台员工为受试者添加业务标签（如"高危人群"、"重点关注"）
	AddTag(ctx context.Context, testeeID uint64, tag string) error

	// RemoveTag 移除业务标签
	RemoveTag(ctx context.Context, testeeID uint64, tag string) error

	// MarkAsKeyFocus 标记为重点关注
	// 场景：将需要特别关注的受试者标记出来
	MarkAsKeyFocus(ctx context.Context, testeeID uint64) error

	// UnmarkKeyFocus 取消重点关注
	UnmarkKeyFocus(ctx context.Context, testeeID uint64) error
}

// TesteeQueryService 受试者查询服务（只读）
// 行为者：所有需要查询受试者信息的用户
// 职责：提供受试者信息查询能力
// 变更来源：查询需求变化（通常较少变更）
type TesteeQueryService interface {
	// GetByID 根据ID查询受试者
	GetByID(ctx context.Context, testeeID uint64) (*TesteeResult, error)

	// FindByProfile 根据用户档案ID查询受试者
	FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error)

	// ListTestees 列出受试者（后台管理）
	ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error)

	// ListKeyFocus 列出重点关注的受试者
	ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) (*TesteeListResult, error)

	// ListByProfileIDs 根据多个用户档案ID查询受试者列表
	// 用于查询当前用户（监护人）的所有受试者
	ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) (*TesteeListResult, error)
}

// ============= DTOs =============

// RegisterTesteeDTO 注册受试者 DTO
type RegisterTesteeDTO struct {
	OrgID     int64      // 机构ID
	ProfileID *uint64    // 用户档案ID（可选，当前对应 IAM.Child.ID）
	Name      string     // 姓名
	Gender    int8       // 性别（0-未知,1-男,2-女）
	Birthday  *time.Time // 出生日期
	Source    string     // 数据来源："online_form" / "wechat_miniprogram"
}

// EnsureTesteeDTO 确保受试者存在 DTO（幂等）
type EnsureTesteeDTO struct {
	OrgID     int64      // 机构ID
	ProfileID *uint64    // 用户档案ID
	Name      string     // 姓名
	Gender    int8       // 性别
	Birthday  *time.Time // 出生日期
}

// UpdateTesteeProfileDTO 更新受试者档案 DTO
type UpdateTesteeProfileDTO struct {
	TesteeID uint64     // 受试者ID
	Name     string     // 姓名
	Gender   int8       // 性别
	Birthday *time.Time // 出生日期
}

// ListTesteeDTO 列出受试者 DTO
type ListTesteeDTO struct {
	OrgID    int64    // 机构ID
	Name     string   // 姓名（模糊搜索）
	Tags     []string // 标签过滤
	KeyFocus *bool    // 是否重点关注
	Offset   int      // 偏移量
	Limit    int      // 限制数量
}

// TesteeResult 受试者结果 DTO
type TesteeResult struct {
	ID         uint64     // 受试者ID
	OrgID      int64      // 机构ID
	ProfileID  *uint64    // 用户档案ID
	Name       string     // 姓名
	Gender     int8       // 性别
	Birthday   *time.Time // 出生日期
	Age        int        // 年龄
	Tags       []string   // 业务标签
	Source     string     // 数据来源
	IsKeyFocus bool       // 是否重点关注

	// 统计信息（仅后台管理需要）
	LastAssessmentAt *time.Time // 最近测评时间
	TotalAssessments int        // 总测评次数
	LastRiskLevel    string     // 最近风险等级
}

// TesteeListResult 受试者列表结果 DTO
type TesteeListResult struct {
	Items      []*TesteeResult // 受试者列表
	TotalCount int64           // 总数
	Offset     int             // 偏移量
	Limit      int             // 限制数量
}
