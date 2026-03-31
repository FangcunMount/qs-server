package operator

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 操作者查询服务实现
// 行为者：所有需要查询后台操作者信息的用户
type queryService struct {
	repo domain.Repository
}

// NewQueryService 创建操作者查询服务
func NewQueryService(repo domain.Repository) OperatorQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByID 根据ID查询操作者
func (s *queryService) GetByID(ctx context.Context, operatorID uint64) (*OperatorResult, error) {
	st, err := s.repo.FindByID(ctx, domain.ID(operatorID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find operator")
	}

	return toOperatorResult(st), nil
}

// GetByUser 根据用户ID查询操作者
func (s *queryService) GetByUser(ctx context.Context, orgID int64, userID int64) (*OperatorResult, error) {
	st, err := s.repo.FindByUser(ctx, orgID, userID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, errors.Wrap(err, "failed to find operator by user")
	}

	return toOperatorResult(st), nil
}

// ListOperators 列出操作者
func (s *queryService) ListOperators(ctx context.Context, dto ListOperatorDTO) (*OperatorListResult, error) {
	var operators []*domain.Operator
	var err error

	if dto.Role != "" {
		role := domain.Role(dto.Role)
		operators, err = s.repo.ListByRole(ctx, dto.OrgID, role, dto.Offset, dto.Limit)
	} else {
		operators, err = s.repo.ListByOrg(ctx, dto.OrgID, dto.Offset, dto.Limit)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list operators")
	}

	// 获取总数
	totalCount, err := s.repo.Count(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count operators")
	}

	// 转换为 DTO
	items := make([]*OperatorResult, len(operators))
	for i, item := range operators {
		items[i] = toOperatorResult(item)
	}

	return &OperatorListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}
