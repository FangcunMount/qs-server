package shared

import (
	"context"
	"time"
)

// ============= Management 应用服务接口（Driving Ports）=============

// TesteeProfileApplicationService 受试者档案应用服务
// 操作者：后台员工（Staff）
// 场景：后台管理系统编辑受试者基本信息
type TesteeProfileApplicationService interface {
	// UpdateBasicInfo 更新基本信息（姓名、性别、生日）
	UpdateBasicInfo(ctx context.Context, dto UpdateTesteeProfileDTO) error

	// BindProfile 绑定用户档案
	// 场景：临时受试者补绑正式档案
	// 注意：当前 profileID 对应 IAM.Child.ID
	BindProfile(ctx context.Context, testeeID uint64, profileID uint64) error
}

// TesteeTagApplicationService 受试者标签应用服务
// 操作者：后台员工（Staff）
// 场景：管理受试者业务标签
type TesteeTagApplicationService interface {
	// AddTag 添加标签
	AddTag(ctx context.Context, testeeID uint64, tag string) error

	// RemoveTag 移除标签
	RemoveTag(ctx context.Context, testeeID uint64, tag string) error

	// MarkAsKeyFocus 标记为重点关注
	MarkAsKeyFocus(ctx context.Context, testeeID uint64) error

	// UnmarkKeyFocus 取消重点关注
	UnmarkKeyFocus(ctx context.Context, testeeID uint64) error
}

// TesteeQueryApplicationService 受试者查询应用服务（只读）
// 操作者：后台员工（Staff）
// 场景：后台管理系统查看受试者档案
type TesteeQueryApplicationService interface {
	// GetByID 根据ID查询受试者
	GetByID(ctx context.Context, testeeID uint64) (*TesteeManagementResult, error)

	// FindByProfile 根据用户档案 ID 查询受试者
	// 注意：当前 profileID 对应 IAM.Child.ID
	FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeManagementResult, error)

	// ListTestees 列出受试者
	ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error)

	// ListKeyFocus 列出重点关注的受试者
	ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) (*TesteeListResult, error)
}

// ============= Registration 应用服务接口（Driving Ports）=============

// TesteeRegistrationApplicationService 受试者注册应用服务
// 操作者：C端用户（患者/家长）
// 场景：首次使用测评系统，注册为受试者
type TesteeRegistrationApplicationService interface {
	// Register 注册受试者
	// 场景：患者/家长首次使用测评系统
	Register(ctx context.Context, dto RegisterTesteeDTO) (*TesteeResult, error)

	// EnsureByProfile 确保受试者存在（幂等）
	// 场景：测评计划创建、答卷提交前
	// 注意：当前 profileID 对应 IAM.Child.ID
	EnsureByProfile(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error)
}

// TesteeProfileQueryApplicationService C端受试者档案查询应用服务（只读）
// 操作者：C端用户（患者/家长）
// 场景：查看自己的测评档案
type TesteeProfileQueryApplicationService interface {
	// GetByProfile 根据用户档案ID获取受试者档案
	// 注意：当前 profileID 对应 IAM.Child.ID
	GetByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error)
}

// ============= Management DTOs =============

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

// TesteeManagementResult 受试者管理结果 DTO
type TesteeManagementResult struct {
	ID         uint64     // 受试者ID
	OrgID      int64      // 机构ID
	ProfileID  *uint64    // 用户档案ID（当前对应 IAM.Child.ID）
	Name       string     // 姓名
	Gender     int8       // 性别
	Birthday   *time.Time // 出生日期
	Age        int        // 年龄
	Tags       []string   // 业务标签
	Source     string     // 数据来源
	IsKeyFocus bool       // 是否重点关注

	// 统计信息
	LastAssessmentAt *time.Time // 最近测评时间
	TotalAssessments int        // 总测评次数
	LastRiskLevel    string     // 最近风险等级
}

// TesteeListResult 受试者列表结果 DTO
type TesteeListResult struct {
	Items      []*TesteeManagementResult // 受试者列表
	TotalCount int64                     // 总数
	Offset     int                       // 偏移量
	Limit      int                       // 限制数量
}

// ============= Registration DTOs =============

// RegisterTesteeDTO 注册受试者 DTO
type RegisterTesteeDTO struct {
	OrgID     int64      // 机构ID
	ProfileID *uint64    // 用户档案ID（当前对应 IAM.Child.ID）
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

// TesteeResult 受试者结果 DTO
type TesteeResult struct {
	ID        uint64     // 受试者ID
	OrgID     int64      // 机构ID
	ProfileID *uint64    // 用户档案ID
	Name      string     // 姓名
	Gender    int8       // 性别
	Birthday  *time.Time // 出生日期
	Age       int        // 年龄
	Source    string     // 数据来源
}

// ============= Composite Service Interface =============

// Service 是 Testee 模块的聚合服务接口
// 它聚合了多个细粒度的应用服务，为 Handler 层提供统一的入口
type Service interface {
	// Create 创建受试者（从 Registration 服务）
	Create(ctx context.Context, dto CreateTesteeDTO) (*CompositeTesteeResult, error)

	// GetByID 获取受试者详情（从 Query 服务）
	GetByID(ctx context.Context, testeeID uint64) (*CompositeTesteeResult, error)

	// Update 更新受试者（聚合 Profile + Tag 服务）
	Update(ctx context.Context, testeeID uint64, dto UpdateTesteeDTO) (*CompositeTesteeResult, error)

	// Delete 删除受试者
	Delete(ctx context.Context, testeeID uint64) error

	// FindByName 根据姓名查找受试者（从 Query 服务）
	FindByName(ctx context.Context, orgID int64, name string) ([]*CompositeTesteeResult, error)

	// ListByTags 根据标签列表查询（从 Query 服务）
	ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*CompositeTesteeResult, error)

	// ListKeyFocus 查询重点关注的受试者（从 Query 服务）
	ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*CompositeTesteeResult, error)

	// ListByOrg 查询机构下所有受试者（从 Query 服务）
	ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*CompositeTesteeResult, error)

	// CountByOrg 统计机构下的受试者数量（从 Query 服务）
	CountByOrg(ctx context.Context, orgID int64) (int64, error)

	// FindByProfileID 根据用户档案 ID 查找受试者（从 Query 服务）
	FindByProfileID(ctx context.Context, orgID int64, profileID uint64) (*CompositeTesteeResult, error)
}

// ============= Composite Service DTOs =============

// CreateTesteeDTO 创建受试者 DTO
type CreateTesteeDTO struct {
	OrgID      int64      // 机构ID
	ProfileID  *uint64    // 用户档案ID（可选，当前对应 IAM.Child.ID）
	Name       string     // 姓名
	Gender     int8       // 性别
	Birthday   *time.Time // 出生日期
	Tags       []string   // 业务标签
	Source     string     // 数据来源
	IsKeyFocus bool       // 是否重点关注
}

// UpdateTesteeDTO 更新受试者 DTO
type UpdateTesteeDTO struct {
	Name       *string    // 姓名（可选）
	Gender     *int8      // 性别（可选）
	Birthday   *time.Time // 出生日期（可选）
	Tags       []string   // 业务标签（可选，完整替换）
	IsKeyFocus *bool      // 是否重点关注（可选）
}

// CompositeTesteeResult 受试者结果 DTO（聚合服务使用）
type CompositeTesteeResult struct {
	ID         uint64     // 受试者ID
	OrgID      int64      // 机构ID
	ProfileID  *uint64    // 用户档案ID（当前对应 IAM.Child.ID）
	Name       string     // 姓名
	Gender     int8       // 性别
	Birthday   *time.Time // 出生日期
	Age        int        // 年龄
	Tags       []string   // 业务标签
	Source     string     // 数据来源
	IsKeyFocus bool       // 是否重点关注

	// 统计信息
	AssessmentStats *AssessmentStatsResult // 测评统计
}

// AssessmentStatsResult 测评统计结果
type AssessmentStatsResult struct {
	LastAssessmentAt *time.Time // 最近测评时间
	TotalAssessments int        // 总测评次数
	LastRiskLevel    string     // 最近风险等级
}
