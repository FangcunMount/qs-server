package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Lifecycle 问卷生命周期管理接口
// 作为领域服务，负责：
// 1. 业务规则验证
// 2. 流程编排（如版本管理）
// 3. 调用聚合根的包内方法完成状态变更和事件触发
type Lifecycle interface {
	// Publish 发布问卷，将草稿状态的问卷变更为已发布状态
	Publish(ctx context.Context, q *Questionnaire) error
	// Unpublish 下线问卷，将已发布的问卷变更为草稿状态
	Unpublish(ctx context.Context, q *Questionnaire) error
	// Archive 归档问卷，将问卷变更为已归档状态
	Archive(ctx context.Context, q *Questionnaire) error
}

// lifecycle 问卷生命周期管理实现
type lifecycle struct{}

// NewLifecycle 创建生命周期管理器
func NewLifecycle() Lifecycle {
	return &lifecycle{}
}

// 保证实现接口
var _ Lifecycle = (*lifecycle)(nil)

// Publish 发布问卷，将草稿状态的问卷变更为已发布状态
func (l *lifecycle) Publish(ctx context.Context, q *Questionnaire) error {
	// 1. 前置状态检查
	if q.IsArchived() {
		return errors.WithCode(code.ErrQuestionnaireArchived, "archived questionnaire cannot be published")
	}
	if q.IsPublished() {
		return errors.WithCode(code.ErrQuestionnaireInvalidStatus, "questionnaire is already published")
	}

	// 2. 业务规则验证
	validator := Validator{}
	validationErrors := validator.ValidateForPublish(q)
	if len(validationErrors) > 0 {
		return ToError(validationErrors)
	}

	// 3. 版本管理：发布时递增大版本号
	versioning := Versioning{}
	if err := versioning.IncrementMajorVersion(q); err != nil {
		return err
	}

	// 4. 调用聚合根的包内方法（状态变更 + 事件触发）
	return q.publish()
}

// Unpublish 下线问卷，将已发布的问卷变更为草稿状态
func (l *lifecycle) Unpublish(ctx context.Context, q *Questionnaire) error {
	// 1. 前置状态检查
	if q.IsArchived() {
		return errors.WithCode(code.ErrQuestionnaireArchived, "questionnaire is already archived")
	}
	if !q.IsPublished() {
		return errors.WithCode(code.ErrQuestionnaireInvalidStatus, "questionnaire is not published")
	}

	// 2. 调用聚合根的包内方法（状态变更 + 事件触发）
	return q.unpublish()
}

// Archive 归档问卷，将问卷变更为已归档状态
func (l *lifecycle) Archive(ctx context.Context, q *Questionnaire) error {
	// 1. 前置状态检查
	if q.IsArchived() {
		return errors.WithCode(code.ErrQuestionnaireArchived, "questionnaire is already archived")
	}

	// 2. 调用聚合根的包内方法（状态变更 + 事件触发）
	return q.archive()
}
