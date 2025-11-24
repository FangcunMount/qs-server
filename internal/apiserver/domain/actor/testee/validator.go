package testee

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Validator 受试者数据验证领域服务
// 负责在创建、修改受试者时进行业务规则验证
type Validator interface {
	// ValidateForCreation 验证创建受试者的数据
	ValidateForCreation(ctx context.Context, orgID int64, name string, gender Gender) error

	// ValidateForUpdate 验证更新受试者的数据
	ValidateForUpdate(ctx context.Context, testee *Testee, name *string, gender *Gender) error

	// ValidateProfileBinding 验证档案绑定
	ValidateProfileBinding(ctx context.Context, testee *Testee, profileID uint64) error

	// ValidateName 验证姓名
	ValidateName(name string, required bool) error

	// ValidateGender 验证性别
	ValidateGender(gender Gender) error

	// ValidateBirthday 验证生日
	ValidateBirthday(birthday *time.Time) error

	// ValidateTag 验证单个标签
	ValidateTag(tag string) error

	// ValidateTags 验证标签列表
	ValidateTags(tags []string) error
}

// validator 验证器实现
type validator struct {
	repo Repository
}

// NewValidator 创建验证器
func NewValidator(repo Repository) Validator {
	return &validator{
		repo: repo,
	}
}

// ValidateForCreation 验证创建受试者的数据
func (v *validator) ValidateForCreation(ctx context.Context, orgID int64, name string, gender Gender) error {
	// 验证机构ID
	if orgID <= 0 {
		return errors.WithCode(code.ErrInvalidArgument, "org_id must be positive")
	}

	// 验证姓名
	name = strings.TrimSpace(name)
	if err := v.ValidateName(name, true); err != nil {
		return err
	}

	// 验证性别
	if err := v.ValidateGender(gender); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate 验证更新受试者的数据
func (v *validator) ValidateForUpdate(ctx context.Context, testee *Testee, name *string, gender *Gender) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 验证姓名（如果提供）
	if name != nil {
		trimmedName := strings.TrimSpace(*name)
		if err := v.ValidateName(trimmedName, true); err != nil {
			return err
		}
	}

	// 验证性别（如果提供）
	if gender != nil {
		if err := v.ValidateGender(*gender); err != nil {
			return err
		}
	}

	return nil
}

// ValidateProfileBinding 验证档案绑定
func (v *validator) ValidateProfileBinding(ctx context.Context, testee *Testee, profileID uint64) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	if profileID == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "profile_id must be positive")
	}

	// 检查是否已绑定其他档案
	if testee.IsBoundToProfile() {
		currentProfileID := *testee.ProfileID()
		if currentProfileID == profileID {
			// 重复绑定同一个档案，幂等操作，不报错
			return nil
		}
		return errors.WithCode(code.ErrValidation, "testee already bound to another profile")
	}

	// 检查该档案是否已被其他受试者使用
	existingTestee, err := v.repo.FindByProfile(ctx, testee.OrgID(), profileID)
	if err != nil {
		// 如果是未找到错误，说明该档案未被使用，可以绑定
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil
		}
		return err
	}

	// 如果找到了，且不是当前受试者，则档案已被占用
	if existingTestee.ID() != testee.ID() {
		return errors.WithCode(code.ErrUserAlreadyExists, "profile already bound to another testee")
	}

	return nil
}

// ValidateOrgID 验证机构ID（已废弃，保留用于兼容）
func (v *validator) ValidateOrgID(orgID int64) error {
	if orgID <= 0 {
		return errors.WithCode(code.ErrValidation, "orgID must be positive")
	}
	return nil
}

// ValidateName 验证姓名
func (v *validator) ValidateName(name string, required bool) error {
	if required && name == "" {
		return errors.WithCode(code.ErrValidation, "name cannot be empty")
	}

	if name != "" && len(name) > 100 {
		return errors.WithCode(code.ErrValidation, "name too long (max 100 characters)")
	}

	return nil
}

// ValidateGender 验证性别
func (v *validator) ValidateGender(gender Gender) error {
	// 验证性别枚举值
	if gender != GenderUnknown && gender != GenderMale && gender != GenderFemale {
		return errors.WithCode(code.ErrValidation, "invalid gender value")
	}
	return nil
}

// ValidateBirthday 验证生日
func (v *validator) ValidateBirthday(birthday *time.Time) error {
	if birthday == nil || birthday.IsZero() {
		return nil // 允许为空
	}

	now := time.Now()

	// 不能是未来时间
	if birthday.After(now) {
		return errors.WithCode(code.ErrValidation, "birthday cannot be in the future")
	}

	// 不能超过150岁
	age := now.Year() - birthday.Year()
	if age > 150 {
		return errors.WithCode(code.ErrValidation, "birthday is too old (max 150 years)")
	}

	// 不能是负年龄
	if age < 0 {
		return errors.WithCode(code.ErrValidation, "invalid birthday")
	}

	return nil
}

// ValidateTag 验证单个标签
func (v *validator) ValidateTag(tag string) error {
	if tag == "" {
		return errors.WithCode(code.ErrValidation, "tag cannot be empty")
	}

	if len(tag) > 50 {
		return errors.WithCode(code.ErrValidation, "tag too long (max 50 characters)")
	}

	// 可以添加更多标签格式验证，如不能包含特殊字符等
	// 规则：只允许字母、数字、下划线、中文
	if !regexp.MustCompile(`^[\w\p{Han}]+$`).MatchString(tag) {
		return errors.WithCode(code.ErrValidation, "tag contains invalid characters")
	}

	return nil
}

// ValidateTags 验证标签列表
func (v *validator) ValidateTags(tags []string) error {
	if len(tags) > 50 {
		return errors.WithCode(code.ErrValidation, "too many tags (max 50)")
	}

	// 验证每个标签
	for _, tag := range tags {
		if err := v.ValidateTag(tag); err != nil {
			return errors.Wrapf(err, "invalid tag: %s", tag)
		}
	}

	return nil
}
