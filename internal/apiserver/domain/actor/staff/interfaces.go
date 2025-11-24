package staff

import "context"

// Repository 员工仓储接口
type Repository interface {
	// Save 保存员工
	Save(ctx context.Context, staff *Staff) error

	// Update 更新员工
	Update(ctx context.Context, staff *Staff) error

	// FindByID 根据ID查找员工
	FindByID(ctx context.Context, id ID) (*Staff, error)

	// FindByUser 根据用户ID查找员工
	FindByUser(ctx context.Context, orgID int64, userID int64) (*Staff, error)

	// ListByOrg 列出机构下的员工
	ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*Staff, error)

	// ListByRole 根据角色查找员工
	ListByRole(ctx context.Context, orgID int64, role Role, offset, limit int) ([]*Staff, error)

	// Delete 删除员工
	Delete(ctx context.Context, id ID) error

	// Count 统计机构下的员工数量
	Count(ctx context.Context, orgID int64) (int64, error)
}

// Factory 员工工厂领域服务
type Factory interface {
	// GetOrCreateByUser 根据用户ID获取或创建员工（幂等）
	GetOrCreateByUser(
		ctx context.Context,
		orgID int64,
		userID int64,
		name string,
	) (*Staff, error)
}
