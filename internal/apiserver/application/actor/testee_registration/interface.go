package testee_registration

import (
	"context"
	"time"
)

// ============= 应用服务接口（Driving Ports）=============

// TesteeRegistrationApplicationService 受试者注册应用服务
// 操作者：C端用户（患者/家长）
// 场景：首次使用测评系统，注册为受试者
type TesteeRegistrationApplicationService interface {
	// Register 注册受试者
	// 场景：患者/家长首次使用测评系统
	Register(ctx context.Context, dto RegisterTesteeDTO) (*TesteeResult, error)

	// EnsureByIAMChild 确保儿童受试者存在（幂等）
	// 场景：测评计划创建、答卷提交前
	EnsureByIAMChild(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error)

	// EnsureByIAMUser 确保成人受试者存在（幂等）
	// 场景：成人自测
	EnsureByIAMUser(ctx context.Context, dto EnsureTesteeDTO) (*TesteeResult, error)
}

// TesteeProfileQueryApplicationService 受试者档案查询应用服务（只读）
// 操作者：C端用户（患者/家长）
// 场景：查看自己的测评档案
type TesteeProfileQueryApplicationService interface {
	// GetByIAMUser 根据IAM用户ID获取受试者档案
	GetByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*TesteeResult, error)

	// GetByIAMChild 根据IAM儿童ID获取受试者档案
	GetByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*TesteeResult, error)
}

// ============= DTOs =============

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
