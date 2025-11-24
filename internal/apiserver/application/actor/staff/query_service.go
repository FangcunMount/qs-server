package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 员工查询服务实现
// 行为者：所有需要查询员工信息的用户
type queryService struct {
	repo staff.Repository
}

// NewQueryService 创建员工查询服务
func NewQueryService(repo staff.Repository) StaffQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByID 根据ID查询员工
func (s *queryService) GetByID(ctx context.Context, staffID uint64) (*StaffResult, error) {
	st, err := s.repo.FindByID(ctx, staff.ID(staffID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find staff")
	}

	return toStaffResult(st), nil
}

// GetByUser 根据用户ID查询员工
func (s *queryService) GetByUser(ctx context.Context, orgID int64, userID int64) (*StaffResult, error) {
	st, err := s.repo.FindByUser(ctx, orgID, userID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "staff not found")
		}
		return nil, errors.Wrap(err, "failed to find staff by user")
	}

	return toStaffResult(st), nil
}

// ListStaffs 列出员工
func (s *queryService) ListStaffs(ctx context.Context, dto ListStaffDTO) (*StaffListResult, error) {
	var staffs []*staff.Staff
	var err error

	if dto.Role != "" {
		role := staff.Role(dto.Role)
		staffs, err = s.repo.ListByRole(ctx, dto.OrgID, role, dto.Offset, dto.Limit)
	} else {
		staffs, err = s.repo.ListByOrg(ctx, dto.OrgID, dto.Offset, dto.Limit)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list staffs")
	}

	// 获取总数
	totalCount, err := s.repo.Count(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count staffs")
	}

	// 转换为 DTO
	items := make([]*StaffResult, len(staffs))
	for i, st := range staffs {
		items[i] = toStaffResult(st)
	}

	return &StaffListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}
