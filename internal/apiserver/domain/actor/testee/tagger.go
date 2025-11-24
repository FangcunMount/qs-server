package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Tagger 标签管理领域服务
// 负责受试者标签的添加、移除和清理
type Tagger interface {
	// Tag 给受试者打标签
	Tag(ctx context.Context, testee *Testee, tag Tag) error

	// UnTag 移除受试者的标签
	UnTag(ctx context.Context, testee *Testee, tag Tag) error

	// CleanTag 清空受试者的所有标签
	CleanTag(ctx context.Context, testee *Testee) error
}

// tagger 标签管理器实现
type tagger struct {
	validator Validator
}

// NewTagger 创建标签管理器
func NewTagger(validator Validator) Tagger {
	return &tagger{
		validator: validator,
	}
}

// Tag 给受试者打标签
func (t *tagger) Tag(ctx context.Context, testee *Testee, tag Tag) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 验证标签格式
	if err := t.validator.ValidateTag(string(tag)); err != nil {
		return err
	}

	// 检查是否已存在该标签（幂等操作）
	if testee.HasTag(tag) {
		return nil
	}

	// 添加标签
	testee.addTag(tag)

	return nil
}

// UnTag 移除受试者的标签
func (t *tagger) UnTag(ctx context.Context, testee *Testee, tag Tag) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 移除标签（如果不存在也不报错，幂等操作）
	testee.removeTag(tag)

	return nil
}

// CleanTag 清空受试者的所有标签
func (t *tagger) CleanTag(ctx context.Context, testee *Testee) error {
	if testee == nil {
		return errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	// 清空标签
	testee.tags = make([]Tag, 0)

	return nil
}
