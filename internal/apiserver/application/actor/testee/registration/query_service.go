package registration

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 受试者档案查询服务实现
type queryService struct {
	repo testee.Repository
}

// NewQueryService 创建受试者档案查询服务
func NewQueryService(repo testee.Repository) shared.TesteeProfileQueryApplicationService {
	return &queryService{
		repo: repo,
	}
}

// GetByProfile 根据用户档案ID获取受试者档案
func (s *queryService) GetByProfile(ctx context.Context, orgID int64, profileID uint64) (*shared.TesteeResult, error) {
	t, err := s.repo.FindByProfile(ctx, orgID, profileID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	return toTesteeResult(t), nil
}
