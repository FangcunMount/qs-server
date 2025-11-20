package staff_management

import (
	"context"
)

// compositeService 是聚合服务的实现
// 它组合多个细粒度的应用服务，提供统一的接口
type compositeService struct {
	staffService   StaffApplicationService
	profileService StaffProfileApplicationService
	roleService    StaffRoleApplicationService
	queryService   StaffQueryApplicationService
}

// NewCompositeService 创建聚合服务
func NewCompositeService(
	staffService StaffApplicationService,
	profileService StaffProfileApplicationService,
	roleService StaffRoleApplicationService,
	queryService StaffQueryApplicationService,
) Service {
	return &compositeService{
		staffService:   staffService,
		profileService: profileService,
		roleService:    roleService,
		queryService:   queryService,
	}
}

// Register 注册新员工
func (s *compositeService) Register(ctx context.Context, dto RegisterStaffDTO) (*StaffResult, error) {
	return s.staffService.Register(ctx, dto)
}

// GetByID 获取员工详情
func (s *compositeService) GetByID(ctx context.Context, staffID uint64) (*StaffResult, error) {
	return s.queryService.GetByID(ctx, staffID)
}

// Delete 删除员工
func (s *compositeService) Delete(ctx context.Context, staffID uint64) error {
	return s.staffService.Delete(ctx, staffID)
}

// ListByRole 根据角色查询员工
func (s *compositeService) ListByRole(ctx context.Context, orgID int64, role string, offset, limit int) ([]*StaffResult, error) {
	listDTO := ListStaffDTO{
		OrgID:  orgID,
		Role:   role,
		Offset: offset,
		Limit:  limit,
	}

	listResult, err := s.queryService.ListStaffs(ctx, listDTO)
	if err != nil {
		return nil, err
	}

	return listResult.Items, nil
}

// ListByOrg 查询机构下所有员工
func (s *compositeService) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*StaffResult, error) {
	listDTO := ListStaffDTO{
		OrgID:  orgID,
		Offset: offset,
		Limit:  limit,
	}

	listResult, err := s.queryService.ListStaffs(ctx, listDTO)
	if err != nil {
		return nil, err
	}

	return listResult.Items, nil
}

// CountByOrg 统计机构下的员工数量
func (s *compositeService) CountByOrg(ctx context.Context, orgID int64) (int64, error) {
	listDTO := ListStaffDTO{
		OrgID:  orgID,
		Offset: 0,
		Limit:  1, // 只需要总数
	}

	listResult, err := s.queryService.ListStaffs(ctx, listDTO)
	if err != nil {
		return 0, err
	}

	return listResult.TotalCount, nil
}
