package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Lifecycle 量表生命周期管理接口
// 作为领域服务，负责：
// 1. 业务规则验证
// 2. 流程编排
// 3. 调用聚合根的包内方法完成状态变更和事件触发
type Lifecycle interface {
	// Publish 发布量表，将草稿状态的量表变更为已发布状态
	Publish(ctx context.Context, scale *MedicalScale) error
	// Unpublish 下线量表，将已发布的量表变更为草稿状态
	Unpublish(ctx context.Context, scale *MedicalScale) error
	// Archive 归档量表，将量表变更为已归档状态
	Archive(ctx context.Context, scale *MedicalScale) error
}

// lifecycle 量表生命周期管理实现
type lifecycle struct{}

// NewLifecycle 创建生命周期管理器
func NewLifecycle() Lifecycle {
	return &lifecycle{}
}

// 保证实现接口
var _ Lifecycle = (*lifecycle)(nil)

// Publish 发布量表
func (l *lifecycle) Publish(ctx context.Context, scale *MedicalScale) error {
	// 1. 前置状态检查
	if scale.IsArchived() {
		return errors.WithCode(code.ErrInvalidArgument, "archived scale cannot be published")
	}
	if scale.IsPublished() {
		return errors.WithCode(code.ErrInvalidArgument, "scale is already published")
	}

	// 2. 业务规则验证
	validator := Validator{}
	validationErrors := validator.ValidateForPublish(scale)
	if len(validationErrors) > 0 {
		return ToError(validationErrors)
	}

	// 3. 调用聚合根的包内方法（状态变更 + 事件触发）
	return scale.publish()
}

// Unpublish 下线量表
func (l *lifecycle) Unpublish(ctx context.Context, scale *MedicalScale) error {
	// 1. 前置状态检查
	if scale.IsArchived() {
		return errors.WithCode(code.ErrInvalidArgument, "archived scale cannot be unpublished")
	}
	if !scale.IsPublished() {
		return errors.WithCode(code.ErrInvalidArgument, "scale is not published")
	}

	// 2. 调用聚合根的包内方法（状态变更 + 事件触发）
	return scale.unpublish()
}

// Archive 归档量表
func (l *lifecycle) Archive(ctx context.Context, scale *MedicalScale) error {
	// 1. 前置状态检查
	if scale.IsArchived() {
		return errors.WithCode(code.ErrInvalidArgument, "scale is already archived")
	}

	// 2. 调用聚合根的包内方法（状态变更 + 事件触发）
	return scale.archive()
}
