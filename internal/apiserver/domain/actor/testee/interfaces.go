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

	// FindByIAMUser 根据IAM用户ID查找受试者
	FindByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*Testee, error)

	// FindByIAMChild 根据IAM儿童ID查找受试者
	FindByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*Testee, error)

	// FindByOrgAndName 根据机构和姓名查找受试者列表（用于模糊匹配）
	FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*Testee, error)

	// ListByOrg 列出机构下的受试者
	ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*Testee, error)

	// ListByTags 根据标签查找受试者
	ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*Testee, error)

	// ListKeyFocus 列出重点关注的受试者
	ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*Testee, error)

	// Delete 删除受试者（软删除）
	Delete(ctx context.Context, id ID) error

	// Count 统计机构下的受试者数量
	Count(ctx context.Context, orgID int64) (int64, error)
}

// Factory 受试者工厂领域服务
type Factory interface {
	// GetOrCreateByIAMChild 根据IAM儿童ID获取或创建受试者
	GetOrCreateByIAMChild(
		ctx context.Context,
		orgID int64,
		iamChildID int64,
		name string,
		gender int8,
		birthday *time.Time,
	) (*Testee, error)

	// GetOrCreateByIAMUser 根据IAM用户ID获取或创建受试者
	GetOrCreateByIAMUser(
		ctx context.Context,
		orgID int64,
		iamUserID int64,
		name string,
		gender int8,
		birthday *time.Time,
	) (*Testee, error)

	// CreateTemporary 创建临时受试者（不绑定IAM）
	CreateTemporary(
		ctx context.Context,
		orgID int64,
		name string,
		gender int8,
		birthday *time.Time,
		source string,
	) (*Testee, error)
}
