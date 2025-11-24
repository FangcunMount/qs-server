package testee_management

import (
	"context"
	"time"
)

// Service 是 Testee 模块的聚合服务接口
// 它聚合了多个细粒度的应用服务，为 Handler 层提供统一的入口
type Service interface {
	// Create 创建受试者（从 Registration 服务）
	Create(ctx context.Context, dto CreateTesteeDTO) (*TesteeResult, error)

	// GetByID 获取受试者详情（从 Query 服务）
	GetByID(ctx context.Context, testeeID uint64) (*TesteeResult, error)

	// Update 更新受试者（聚合 Profile + Tag 服务）
	Update(ctx context.Context, testeeID uint64, dto UpdateTesteeDTO) (*TesteeResult, error)

	// Delete 删除受试者
	Delete(ctx context.Context, testeeID uint64) error

	// FindByName 根据姓名查找受试者（从 Query 服务）
	FindByName(ctx context.Context, orgID int64, name string) ([]*TesteeResult, error)

	// ListByTags 根据标签列表查询（从 Query 服务）
	ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*TesteeResult, error)

	// ListKeyFocus 查询重点关注的受试者（从 Query 服务）
	ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*TesteeResult, error)

	// ListByOrg 查询机构下所有受试者（从 Query 服务）
	ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*TesteeResult, error)

	// CountByOrg 统计机构下的受试者数量（从 Query 服务）
	CountByOrg(ctx context.Context, orgID int64) (int64, error)

	// FindByProfileID 根据用户档案 ID 查找受试者（从 Query 服务）
	FindByProfileID(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error)
}

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

// TesteeResult 受试者结果 DTO
type TesteeResult struct {
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
