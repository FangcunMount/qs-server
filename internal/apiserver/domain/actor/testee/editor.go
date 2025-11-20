package testee

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Editor 受试者编辑器领域服务
// 负责 Testee 信息的变更，包含业务规则验证
type Editor interface {
	// UpdateBasicInfo 更新基本信息
	UpdateBasicInfo(testee *Testee, name string, gender Gender, birthday *time.Time) error

	// AddTag 添加标签
	AddTag(testee *Testee, tag string) error

	// RemoveTag 移除标签
	RemoveTag(testee *Testee, tag string) error

	// ReplaceTags 替换所有标签
	ReplaceTags(testee *Testee, tags []string) error

	// MarkAsKeyFocus 标记为重点关注
	MarkAsKeyFocus(testee *Testee, reason string) error

	// UnmarkAsKeyFocus 取消重点关注
	UnmarkAsKeyFocus(testee *Testee) error
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
func (e *editor) UpdateBasicInfo(testee *Testee, name string, gender Gender, birthday *time.Time) error {
	// 验证更新参数
	if err := e.validator.ValidateName(name, false); err != nil {
		return err
	}

	if err := e.validator.ValidateGender(gender); err != nil {
		return err
	}

	if err := e.validator.ValidateBirthday(birthday); err != nil {
		return err
	}

	// 执行更新
	testee.updateBasicInfo(name, gender, birthday)

	return nil
}

// AddTag 添加标签
func (e *editor) AddTag(testee *Testee, tag string) error {
	// 验证标签格式
	if err := e.validator.ValidateTag(tag); err != nil {
		return err
	}

	// 检查标签数量限制（例如最多50个标签）
	if len(testee.tags) >= 50 {
		return errors.WithCode(code.ErrValidation, "too many tags (max 50)")
	}

	// 执行添加
	testee.addTag(tag)

	return nil
}

// RemoveTag 移除标签
func (e *editor) RemoveTag(testee *Testee, tag string) error {
	if tag == "" {
		return errors.WithCode(code.ErrInvalidArgument, "tag cannot be empty")
	}

	// 检查标签是否存在
	if !testee.HasTag(tag) {
		return errors.WithCode(code.ErrValidation, "tag not found")
	}

	// 执行移除
	testee.removeTag(tag)

	return nil
}

// ReplaceTags 替换所有标签
func (e *editor) ReplaceTags(testee *Testee, tags []string) error {
	// 验证标签列表
	if err := e.validator.ValidateTags(tags); err != nil {
		return err
	}

	// 清空现有标签
	testee.clearTags()

	// 添加新标签（去重）
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		if !tagSet[tag] {
			testee.addTag(tag)
			tagSet[tag] = true
		}
	}

	return nil
}

// MarkAsKeyFocus 标记为重点关注
func (e *editor) MarkAsKeyFocus(testee *Testee, reason string) error {
	if testee.IsKeyFocus() {
		return nil // 已经是重点关注，幂等操作
	}

	// 可以在这里添加业务规则，比如：
	// - 需要审批
	// - 需要记录原因
	// - 需要发送通知等

	testee.markAsKeyFocus()

	// TODO: 发布领域事件
	// events.Publish(NewTesteeMarkedAsKeyFocusEvent(testee.ID(), reason))

	return nil
}

// UnmarkAsKeyFocus 取消重点关注
func (e *editor) UnmarkAsKeyFocus(testee *Testee) error {
	if !testee.IsKeyFocus() {
		return nil // 本来就不是重点关注，幂等操作
	}

	testee.unmarkAsKeyFocus()

	// TODO: 发布领域事件
	// events.Publish(NewTesteeUnmarkedAsKeyFocusEvent(testee.ID()))

	return nil
}
