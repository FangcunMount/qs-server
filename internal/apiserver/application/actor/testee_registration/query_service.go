package testee_registration

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 受试者档案查询服务实现
type queryService struct {
	repo testee.Repository
}

// NewQueryService 创建受试者档案查询服务
func NewQueryService(repo testee.Repository) TesteeProfileQueryApplicationService {
	return &queryService{
		repo: repo,
	}
}

// GetByIAMUser 根据IAM用户ID获取受试者档案
func (s *queryService) GetByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*TesteeResult, error) {
	t, err := s.repo.FindByIAMUser(ctx, orgID, iamUserID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by iam user")
	}

	return toTesteeResult(t), nil
}

// GetByIAMChild 根据IAM儿童ID获取受试者档案
func (s *queryService) GetByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*TesteeResult, error) {
	t, err := s.repo.FindByIAMChild(ctx, orgID, iamChildID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by iam child")
	}

	return toTesteeResult(t), nil
}
