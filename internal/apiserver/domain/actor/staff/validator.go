package staff

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Validator 员工验证器领域服务
// 按字段维度提供验证方法，可灵活组合
type Validator interface {
	// ValidateOrgID 验证机构ID
	ValidateOrgID(orgID int64) error

	// ValidateUserID 验证用户ID
	ValidateUserID(userID int64) error

	// ValidateName 验证姓名
	// required: 是否必填
	ValidateName(name string, required bool) error

	// ValidateEmail 验证邮箱
	ValidateEmail(email string) error

	// ValidatePhone 验证手机号
	ValidatePhone(phone string) error

	// ValidateRole 验证角色
	ValidateRole(role Role) error

	// ValidateRoles 验证角色列表
	ValidateRoles(roles []Role) error

	// ValidateForCreation 验证创建时的必填字段
	ValidateForCreation(orgID int64, userID int64, name string) error
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

// ValidateUserID 验证用户ID
func (v *validator) ValidateUserID(userID int64) error {
	if userID <= 0 {
		return errors.WithCode(code.ErrValidation, "userID must be positive")
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

// ValidateEmail 验证邮箱
func (v *validator) ValidateEmail(email string) error {
	if email == "" {
		return nil // 允许为空
	}

	// 简单的邮箱格式验证
	if len(email) > 255 {
		return errors.WithCode(code.ErrValidation, "email too long (max 255 characters)")
	}

	// 基本格式检查：包含 @ 和 .
	hasAt := false
	hasDot := false
	for i, c := range email {
		if c == '@' {
			if hasAt {
				return errors.WithCode(code.ErrValidation, "email contains multiple @")
			}
			hasAt = true
			if i == 0 || i == len(email)-1 {
				return errors.WithCode(code.ErrValidation, "invalid email format")
			}
		}
		if c == '.' && hasAt {
			hasDot = true
		}
	}

	if !hasAt || !hasDot {
		return errors.WithCode(code.ErrValidation, "invalid email format")
	}

	return nil
}

// ValidatePhone 验证手机号
func (v *validator) ValidatePhone(phone string) error {
	if phone == "" {
		return nil // 允许为空
	}

	if len(phone) < 7 || len(phone) > 20 {
		return errors.WithCode(code.ErrValidation, "phone length must be between 7 and 20")
	}

	// 检查是否只包含数字、空格、+、-、()
	for _, c := range phone {
		if !(c >= '0' && c <= '9') && c != '+' && c != '-' && c != ' ' && c != '(' && c != ')' {
			return errors.WithCode(code.ErrValidation, "phone contains invalid characters")
		}
	}

	return nil
}

// ValidateRole 验证角色
func (v *validator) ValidateRole(role Role) error {
	// 验证角色是否是预定义的
	switch role {
	case RoleScaleAdmin, RoleEvaluator, RoleScreeningOwner, RoleReportAuditor:
		return nil
	default:
		return errors.WithCode(code.ErrValidation, "invalid role")
	}
}

// ValidateRoles 验证角色列表
func (v *validator) ValidateRoles(roles []Role) error {
	if len(roles) > 20 {
		return errors.WithCode(code.ErrValidation, "too many roles (max 20)")
	}

	// 验证每个角色
	for _, role := range roles {
		if err := v.ValidateRole(role); err != nil {
			return errors.Wrapf(err, "invalid role: %s", role)
		}
	}

	return nil
}

// ValidateForCreation 验证创建时的必填字段
func (v *validator) ValidateForCreation(orgID int64, userID int64, name string) error {
	// 验证机构ID
	if err := v.ValidateOrgID(orgID); err != nil {
		return err
	}

	// 验证用户ID
	if err := v.ValidateUserID(userID); err != nil {
		return err
	}

	// 验证姓名
	if err := v.ValidateName(name, true); err != nil {
		return err
	}

	return nil
}
