package staff

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// factory 员工工厂实现
type factory struct {
	repo      Repository
	validator Validator
}

// NewFactory 创建员工工厂
func NewFactory(repo Repository, validator Validator) Factory {
	return &factory{
		repo:      repo,
		validator: validator,
	}
}

// GetOrCreateByUser 根据用户ID获取或创建员工（幂等）
func (f *factory) GetOrCreateByUser(
	ctx context.Context,
	orgID int64,
	userID int64,
	name string,
) (*Staff, error) {
	// 先尝试查找
	staff, err := f.repo.FindByUser(ctx, orgID, userID)
	if err == nil {
		return staff, nil
	}

	// 如果不存在，创建新的
	if errors.IsCode(err, code.ErrUserNotFound) {
		// 验证创建参数
		if err := f.validator.ValidateForCreation(orgID, userID, name); err != nil {
			return nil, err
		}

		staff = NewStaff(orgID, userID, name)

		if err := f.repo.Save(ctx, staff); err != nil {
			return nil, errors.Wrap(err, "failed to save staff")
		}

		return staff, nil
	}

	return nil, errors.Wrap(err, "failed to find staff by user")
}
