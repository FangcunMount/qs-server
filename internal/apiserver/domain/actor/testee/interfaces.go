package testee

import (
	"context"
	"time"
)

// Repository 受试者仓储接口
type Repository interface {
	// Save 保存受试者
	Save(ctx context.Context, testee *Testee) error

	// Update 更新受试者
	Update(ctx context.Context, testee *Testee) error

	// FindByID 根据ID查找受试者
	FindByID(ctx context.Context, id ID) (*Testee, error)

	// FindByProfile 根据用户档案ID查找受试者
	FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*Testee, error)

	// Delete 删除受试者（软删除）
	Delete(ctx context.Context, id ID) error
}

// Factory 受试者工厂领域服务
type Factory interface {
	// GetOrCreateByProfile 根据用户档案ID获取或创建受试者
	// 注意：当前 profileID 对应 IAM.Child.ID，未来可重构为更通用的档案系统
	GetOrCreateByProfile(
		ctx context.Context,
		orgID int64,
		profileID uint64,
		name string,
		gender int8,
		birthday *time.Time,
	) (*Testee, error)

	// CreateTemporary 创建临时受试者（不绑定档案）
	CreateTemporary(
		ctx context.Context,
		orgID int64,
		name string,
		gender int8,
		birthday *time.Time,
		source string,
	) (*Testee, error)
}
