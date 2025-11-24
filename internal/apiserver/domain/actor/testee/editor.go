package testee

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Editor 受试者信息编辑领域服务
// 负责编辑受试者的基本信息和关注状态
type Editor interface {
	// UpdateBasicInfo 更新基本信息
	UpdateBasicInfo(ctx context.Context, testee *Testee, name *string, gender *Gender, birthday *time.Time) error

	// MarkAsKeyFocus 标记为重点关注
	MarkAsKeyFocus(ctx context.Context, testee *Testee) error

	// UnmarkAsKeyFocus 取消重点关注
	UnmarkAsKeyFocus(ctx context.Context, testee *Testee) error
}

// editor 编辑器实现
type editor struct {
	validator Validator
}

// NewEditor 创建编辑器
func NewEditor(validator Validator) Editor {
	return &editor{
		validator: validator,
	}
}

// UpdateBasicInfo 更新基本信息
func (e *editor) UpdateBasicInfo(ctx context.Context, testee *Testee, name *string, gender *Gender, birthday *time.Time) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 验证数据
	if err := e.validator.ValidateForUpdate(ctx, testee, name, gender); err != nil {
		return err
	}

	// 验证生日
	if birthday != nil {
		if err := e.validator.ValidateBirthday(birthday); err != nil {
			return err
		}
	}

	// 更新姓名
	if name != nil && *name != testee.name {
		testee.name = *name
	}

	// 更新性别
	if gender != nil && *gender != testee.gender {
		testee.gender = *gender
	}

	// 更新生日
	if birthday != nil {
		testee.birthday = birthday
	}

	return nil
}

// MarkAsKeyFocus 标记为重点关注
func (e *editor) MarkAsKeyFocus(ctx context.Context, testee *Testee) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 幂等操作
	if testee.isKeyFocus {
		return nil
	}

	testee.isKeyFocus = true

	return nil
}

// UnmarkAsKeyFocus 取消重点关注
func (e *editor) UnmarkAsKeyFocus(ctx context.Context, testee *Testee) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 幂等操作
	if !testee.isKeyFocus {
		return nil
	}

	testee.isKeyFocus = false

	return nil
}
