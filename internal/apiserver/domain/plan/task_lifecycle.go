package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TaskLifecycle 任务生命周期管理领域服务
// 负责控制单次测评任务：开放任务、完成任务、过期、取消
type TaskLifecycle struct{}

// NewTaskLifecycle 创建任务生命周期管理器
func NewTaskLifecycle() *TaskLifecycle {
	return &TaskLifecycle{}
}

// Open 开放任务（生成入口）
// 将待推送状态的任务变更为已推送状态，并设置入口信息
func (l *TaskLifecycle) Open(ctx context.Context, task *AssessmentTask, entryToken string, entryURL string, expireAt time.Time) error {
	// 1. 前置状态检查
	if !task.IsPending() {
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于待推送状态，无法开放")
	}

	// 2. 验证参数
	if entryToken == "" {
		return errors.WithCode(code.ErrInvalidArgument, "入口令牌不能为空")
	}
	if entryURL == "" {
		return errors.WithCode(code.ErrInvalidArgument, "入口URL不能为空")
	}
	if expireAt.Before(time.Now()) {
		return errors.WithCode(code.ErrInvalidArgument, "过期时间必须在未来")
	}

	// 3. 调用实体的包内方法（状态变更 + 事件触发）
	return task.open(entryToken, entryURL, expireAt)
}

// Complete 完成任务
// 将已推送状态的任务变更为已完成状态，并关联测评记录
func (l *TaskLifecycle) Complete(ctx context.Context, task *AssessmentTask, assessmentID assessment.ID) error {
	// 1. 前置状态检查
	if !task.IsOpened() {
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于已推送状态，无法完成")
	}

	// 2. 验证参数
	if assessmentID.IsZero() {
		return errors.WithCode(code.ErrInvalidArgument, "测评ID不能为空")
	}

	// 3. 调用实体的包内方法（状态变更 + 事件触发）
	return task.complete(assessmentID)
}

// Expire 过期任务
// 将已推送状态的任务变更为已过期状态
func (l *TaskLifecycle) Expire(ctx context.Context, task *AssessmentTask) error {
	// 1. 前置状态检查
	if !task.IsOpened() {
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于已推送状态，无法过期")
	}

	// 2. 调用实体的包内方法（状态变更 + 事件触发）
	return task.expire()
}

// Cancel 取消任务
// 将任务变更为已取消状态（适用于任何非终态任务）
func (l *TaskLifecycle) Cancel(ctx context.Context, task *AssessmentTask) error {
	// 1. 前置状态检查
	if task.IsTerminal() {
		return nil // 幂等操作
	}

	// 2. 调用实体的包内方法（状态变更）
	task.cancel()
	return nil
}
