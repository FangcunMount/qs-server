package testee

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Validator 受试者验证器领域服务
// 按字段维度提供验证方法，可灵活组合
type Validator interface {
	// ValidateOrgID 验证机构ID
	ValidateOrgID(orgID int64) error

	// ValidateName 验证姓名
	// required: 是否必填
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
type validator struct{}

// NewValidator 创建验证器
func NewValidator() Validator {
	return &validator{}
}

// ValidateOrgID 验证机构ID
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
	// 例如：只允许字母、数字、下划线、中文
	// if !regexp.MustCompile(`^[\w\p{Han}]+$`).MatchString(tag) {
	//     return errors.WithCode(code.ErrValidation, "tag contains invalid characters")
	// }

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
