package staff_management

import (
	"context"
)

// Service 是 Staff 模块的聚合服务接口
// 它聚合了多个细粒度的应用服务，为 Handler 层提供统一的入口
type Service interface {
	// Register 注册新员工（从 Staff 服务）
	Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error)

	// GetByID 获取员工详情（从 Query 服务）
	GetByID(ctx context.Context, staffID uint64) (*StaffResult, error)

	// Delete 删除员工（从 Staff 服务）
	Delete(ctx context.Context, staffID uint64) error

	// ListByRole 根据角色查询员工（从 Query 服务）
	ListByRole(ctx context.Context, orgID int64, role string, offset, limit int) ([]*StaffResult, error)

	// ListByOrg 查询机构下所有员工（从 Query 服务）
	ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*StaffResult, error)

	// CountByOrg 统计机构下的员工数量（从 Query 服务）
	CountByOrg(ctx context.Context, orgID int64) (int64, error)
}
