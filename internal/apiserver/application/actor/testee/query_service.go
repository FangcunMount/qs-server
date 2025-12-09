package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 受试者查询服务实现
// 行为者：所有需要查询受试者信息的用户
type queryService struct {
	repo domain.Repository
}

// NewQueryService 创建受试者查询服务
func NewQueryService(repo domain.Repository) TesteeQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByID 根据ID查询受试者
func (s *queryService) GetByID(ctx context.Context, testeeID uint64) (*TesteeResult, error) {
	testee, err := s.repo.FindByID(ctx, domain.ID(testeeID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee")
	}

	return toTesteeResult(testee), nil
}

// FindByProfile 根据用户档案ID查询受试者
func (s *queryService) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error) {
	testee, err := s.repo.FindByProfile(ctx, orgID, profileID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	return toTesteeResult(testee), nil
}

// ListTestees 列出受试者
func (s *queryService) ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error) {
	var testees []*domain.Testee
	var err error

	// 根据不同的过滤条件调用不同的查询方法
	if dto.KeyFocus != nil && *dto.KeyFocus {
		testees, err = s.repo.ListKeyFocus(ctx, dto.OrgID, dto.Offset, dto.Limit)
	} else if len(dto.Tags) > 0 {
		testees, err = s.repo.ListByTags(ctx, dto.OrgID, dto.Tags, dto.Offset, dto.Limit)
	} else if dto.Name != "" {
		// 名称搜索 - 注意：FindByOrgAndName 返回全部结果，需手动分页
		allTestees, findErr := s.repo.FindByOrgAndName(ctx, dto.OrgID, dto.Name)
		if findErr != nil {
			err = findErr
		} else {
			// 手动分页
			start := dto.Offset
			end := dto.Offset + dto.Limit
			if start >= len(allTestees) {
				testees = []*domain.Testee{}
			} else {
				if end > len(allTestees) {
					end = len(allTestees)
				}
				testees = allTestees[start:end]
			}
		}
	} else {
		testees, err = s.repo.ListByOrg(ctx, dto.OrgID, dto.Offset, dto.Limit)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees")
	}

	// 获取总数
	totalCount, err := s.repo.Count(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i, testee := range testees {
		items[i] = toTesteeResult(testee)
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
	testees, err := s.repo.ListKeyFocus(ctx, orgID, offset, limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list key focus testees")
	}

	// 获取总数（这里应该是重点关注的总数，但当前 repo 没有这个方法，暂用全部计数）
	totalCount, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i, testee := range testees {
		items[i] = toTesteeResult(testee)
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
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

	testees, err := s.repo.ListByProfileIDs(ctx, profileIDs, offset, limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees by profile IDs")
	}

	// 注意：这里的总数是所有 profileIDs 的受试者总数
	// 为了精确计数，我们需要单独查询（不带分页）
	allTestees, err := s.repo.ListByProfileIDs(ctx, profileIDs, 0, 999999)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees by profile IDs")
	}
	totalCount := int64(len(allTestees))

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i, testee := range testees {
		items[i] = toTesteeResult(testee)
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}
