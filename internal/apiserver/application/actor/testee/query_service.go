package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 受试者查询服务实现
// 行为者：所有需要查询受试者信息的用户
type queryService struct {
	reader actorreadmodel.TesteeReader
}

// NewQueryService 创建受试者查询服务
func NewQueryService(reader actorreadmodel.TesteeReader) TesteeQueryService {
	return &queryService{
		reader: reader,
	}
}

// GetByID 根据ID查询受试者
func (s *queryService) GetByID(ctx context.Context, testeeID uint64) (*TesteeResult, error) {
	resolvedID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return nil, err
	}

	testee, err := s.reader.GetTestee(ctx, resolvedID.Uint64())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee")
	}

	return toTesteeResultFromRow(testee), nil
}

// FindByProfile 根据用户档案ID查询受试者
func (s *queryService) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error) {
	testee, err := s.reader.FindTesteeByProfile(ctx, orgID, profileID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	return toTesteeResultFromRow(testee), nil
}

// ListTestees 列出受试者
func (s *queryService) ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error) {
	if dto.RestrictToAccessScope {
		for _, id := range dto.AccessibleTesteeIDs {
			if _, err := testeeIDFromUint64("accessible_testee_id", id); err != nil {
				return nil, err
			}
		}
	}

	filter := actorreadmodel.TesteeFilter{
		OrgID:                 dto.OrgID,
		Name:                  dto.Name,
		Tags:                  dto.Tags,
		KeyFocus:              dto.KeyFocus,
		CreatedAtStart:        dto.CreatedAtStart,
		CreatedAtEnd:          dto.CreatedAtEnd,
		AccessibleTesteeIDs:   append([]uint64(nil), dto.AccessibleTesteeIDs...),
		RestrictToAccessScope: dto.RestrictToAccessScope,
		Offset:                dto.Offset,
		Limit:                 dto.Limit,
	}

	testees, err := s.reader.ListTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees")
	}
	totalCount, err := s.reader.CountTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i := range testees {
		items[i] = toTesteeResultFromRow(&testees[i])
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

// ListKeyFocus 列出重点关注的受试者
func (s *queryService) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) (*TesteeListResult, error) {
	keyFocus := true
	return s.ListTestees(ctx, ListTesteeDTO{
		OrgID:    orgID,
		KeyFocus: &keyFocus,
		Offset:   offset,
		Limit:    limit,
	})
}

// ListByProfileIDs 根据多个用户档案ID查询受试者列表
func (s *queryService) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) (*TesteeListResult, error) {
	if len(profileIDs) == 0 {
		return &TesteeListResult{
			Items:      []*TesteeResult{},
			TotalCount: 0,
			Offset:     offset,
			Limit:      limit,
		}, nil
	}

	testees, err := s.reader.ListTesteesByProfileIDs(ctx, profileIDs, offset, limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees by profile IDs")
	}

	totalCount, err := s.reader.CountTesteesByProfileIDs(ctx, profileIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees by profile IDs")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i := range testees {
		items[i] = toTesteeResultFromRow(&testees[i])
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}
