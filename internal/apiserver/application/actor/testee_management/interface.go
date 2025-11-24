package testee_management

import (
	"context"
	"time"
)

// ============= 应用服务接口（Driving Ports）=============

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

// ============= DTOs =============

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
