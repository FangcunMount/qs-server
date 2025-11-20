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

// GetOrCreateByIAMUser 根据IAM用户ID获取或创建员工
func (f *factory) GetOrCreateByIAMUser(
	ctx context.Context,
	orgID int64,
	iamUserID int64,
	name string,
) (*Staff, error) {
	// 先尝试查找
	staff, err := f.repo.FindByIAMUser(ctx, orgID, iamUserID)
	if err == nil {
		return staff, nil
	}

	// 如果不存在，创建新的
	// 使用 ErrUserNotFound 代表记录不存在的情况
	if errors.IsCode(err, code.ErrUserNotFound) {
		// 验证创建参数
		if err := f.validator.ValidateOrgID(orgID); err != nil {
			return nil, err
		}
		if err := f.validator.ValidateIAMUserID(iamUserID); err != nil {
			return nil, err
		}
		if err := f.validator.ValidateName(name, true); err != nil {
			return nil, err
		}

		staff = NewStaff(orgID, iamUserID, name)

		if err := f.repo.Save(ctx, staff); err != nil {
			return nil, errors.Wrap(err, "failed to save staff")
		}

		return staff, nil
	}

	return nil, errors.Wrap(err, "failed to find staff by iam user")
}

// SyncFromIAM 从IAM同步员工信息
// 注意：此方法已过时，建议使用 IAMSynchronizer.SyncBasicInfo
// Deprecated: Use IAMSynchronizer.SyncBasicInfo instead
func (f *factory) SyncFromIAM(ctx context.Context, staff *Staff, name, email, phone string) error {
	// 验证参数
	if err := f.validator.ValidateName(name, false); err != nil {
		return err
	}
	if err := f.validator.ValidateEmail(email); err != nil {
		return err
	}
	if err := f.validator.ValidatePhone(phone); err != nil {
		return err
	}

	staff.updateContactInfo(email, phone)
	// 如果 name 不为空，也更新
	if name != "" {
		staff.name = name
	}

	return f.repo.Update(ctx, staff)
}
