package testee

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// factory 工厂实现
type factory struct {
	repo      Repository
	validator Validator
}

// NewFactory 创建工厂
func NewFactory(repo Repository, validator Validator) Factory {
	return &factory{
		repo:      repo,
		validator: validator,
	}
}

// GetOrCreateByProfile 根据用户档案ID获取或创建受试者
func (f *factory) GetOrCreateByProfile(
	ctx context.Context,
	orgID int64,
	profileID uint64,
	name string,
	gender int8,
	birthday *time.Time,
) (*Testee, error) {
	// 先尝试查找
	testee, err := f.repo.FindByProfile(ctx, orgID, profileID)
	if err == nil {
		return testee, nil
	}

	// 如果不存在，创建新的
	// 使用 ErrUserNotFound 代表记录不存在的情况
	if errors.IsCode(err, code.ErrUserNotFound) {
		// 验证创建参数
		if err := f.validator.ValidateOrgID(orgID); err != nil {
			return nil, err
		}
		if err := f.validator.ValidateName(name, true); err != nil {
			return nil, err
		}
		if err := f.validator.ValidateGender(Gender(gender)); err != nil {
			return nil, err
		}
		if err := f.validator.ValidateBirthday(birthday); err != nil {
			return nil, err
		}

		testee = NewTestee(orgID, name, Gender(gender), birthday)
		testee.bindProfile(profileID)
		testee.SetSource("profile")

		if err := f.repo.Save(ctx, testee); err != nil {
			return nil, errors.Wrap(err, "failed to save testee")
		}

		return testee, nil
	}

	return nil, errors.Wrap(err, "failed to find testee by profile")
}

// CreateTemporary 创建临时受试者（不绑定IAM）
func (f *factory) CreateTemporary(
	ctx context.Context,
	orgID int64,
	name string,
	gender int8,
	birthday *time.Time,
	source string,
) (*Testee, error) {
	// 验证创建参数
	if err := f.validator.ValidateOrgID(orgID); err != nil {
		return nil, err
	}
	if err := f.validator.ValidateName(name, true); err != nil {
		return nil, err
	}
	if err := f.validator.ValidateGender(Gender(gender)); err != nil {
		return nil, err
	}
	if err := f.validator.ValidateBirthday(birthday); err != nil {
		return nil, err
	}

	testee := NewTestee(orgID, name, Gender(gender), birthday)
	testee.SetSource(source)

	if err := f.repo.Save(ctx, testee); err != nil {
		return nil, errors.Wrap(err, "failed to save temporary testee")
	}

	return testee, nil
}
