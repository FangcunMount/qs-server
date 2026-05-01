package operator

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 操作者查询服务实现
// 行为者：所有需要查询后台操作者信息的用户
type queryService struct {
	reader actorreadmodel.OperatorReader
}

// NewQueryService 创建操作者查询服务
func NewQueryService(reader actorreadmodel.OperatorReader) OperatorQueryService {
	return &queryService{
		reader: reader,
	}
}

// GetByID 根据ID查询操作者
func (s *queryService) GetByID(ctx context.Context, operatorID uint64) (*OperatorResult, error) {
	targetOperatorID, err := operatorIDFromUint64("operator_id", operatorID)
	if err != nil {
		return nil, err
	}
	st, err := s.reader.GetOperator(ctx, targetOperatorID.Uint64())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find operator")
	}

	return toOperatorResultFromRow(st), nil
}

// GetByUser 根据用户ID查询操作者
func (s *queryService) GetByUser(ctx context.Context, orgID int64, userID int64) (*OperatorResult, error) {
	st, err := s.reader.FindOperatorByUser(ctx, orgID, userID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, errors.Wrap(err, "failed to find operator by user")
	}

	return toOperatorResultFromRow(st), nil
}

// ListOperators 列出操作者
func (s *queryService) ListOperators(ctx context.Context, dto ListOperatorDTO) (*OperatorListResult, error) {
	operators, err := s.reader.ListOperators(ctx, actorreadmodel.OperatorFilter{
		OrgID:  dto.OrgID,
		Role:   dto.Role,
		Offset: dto.Offset,
		Limit:  dto.Limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list operators")
	}

	// 获取总数
	totalCount, err := s.reader.CountOperators(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count operators")
	}

	// 转换为 DTO
	items := make([]*OperatorResult, len(operators))
	for i := range operators {
		items[i] = toOperatorResultFromRow(&operators[i])
	}

	return &OperatorListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}
