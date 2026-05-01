package operator

import "context"

// Repository 员工仓储接口
type Repository interface {
	// Save 保存员工
	Save(ctx context.Context, staff *Operator) error

	// Update 更新员工
	Update(ctx context.Context, staff *Operator) error

	// FindByID 根据ID查找员工
	FindByID(ctx context.Context, id ID) (*Operator, error)

	// FindByUser 根据用户ID查找员工
	FindByUser(ctx context.Context, orgID int64, userID int64) (*Operator, error)

	// Delete 删除员工
	Delete(ctx context.Context, id ID) error
}

// Factory 员工工厂领域服务
type Factory interface {
	// GetOrCreateByUser 根据用户ID获取或创建员工（幂等）
	GetOrCreateByUser(
		ctx context.Context,
		orgID int64,
		userID int64,
		name string,
	) (*Operator, error)
}
