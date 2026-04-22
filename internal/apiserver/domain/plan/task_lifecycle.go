package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	taskID := task.GetID().String()
	actionAt := time.Now()
	logger.L(ctx).Infow("Opening task in domain service",
		"domain_action", "open_task",
		"task_id", taskID,
		"current_status", task.GetStatus().String(),
		"open_at", actionAt,
		"expire_at", expireAt,
	)

	// 1. 前置状态检查
	if !task.IsPending() {
		logger.L(ctx).Errorw("Task not in pending status",
			"domain_action", "open_task",
			"task_id", taskID,
			"current_status", task.GetStatus().String(),
		)
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于待推送状态，无法开放")
	}

	// 2. 验证参数
	if entryToken == "" {
		return errors.WithCode(code.ErrInvalidArgument, "入口令牌不能为空")
	}
	if entryURL == "" {
		return errors.WithCode(code.ErrInvalidArgument, "入口URL不能为空")
	}
	if expireAt.Before(actionAt) {
		logger.L(ctx).Errorw("Expire time is in the past",
			"domain_action", "open_task",
			"task_id", taskID,
			"open_at", actionAt,
			"expire_at", expireAt,
		)
		return errors.WithCode(code.ErrInvalidArgument, "过期时间必须晚于开放时间")
	}

	// 3. 调用实体的包内方法（状态变更 + 事件触发）
	if err := task.open(entryToken, entryURL, actionAt, expireAt); err != nil {
		logger.L(ctx).Errorw("Failed to open task",
			"domain_action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return err
	}

	logger.L(ctx).Infow("Task opened successfully",
		"domain_action", "open_task",
		"task_id", taskID,
	)

	return nil
}

// Complete 完成任务
// 将已推送状态的任务变更为已完成状态，并关联测评记录
func (l *TaskLifecycle) Complete(ctx context.Context, task *AssessmentTask, assessmentID assessment.ID) error {
	taskID := task.GetID().String()
	actionAt := time.Now()
	logger.L(ctx).Infow("Completing task in domain service",
		"domain_action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID.String(),
		"current_status", task.GetStatus().String(),
		"completed_at", actionAt,
	)

	// 1. 前置状态检查
	if !task.IsOpened() {
		logger.L(ctx).Errorw("Task not in opened status",
			"domain_action", "complete_task",
			"task_id", taskID,
			"current_status", task.GetStatus().String(),
		)
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于已推送状态，无法完成")
	}

	// 2. 验证参数
	if assessmentID.IsZero() {
		return errors.WithCode(code.ErrInvalidArgument, "测评ID不能为空")
	}

	// 3. 调用实体的包内方法（状态变更 + 事件触发）
	if err := task.complete(assessmentID, actionAt); err != nil {
		logger.L(ctx).Errorw("Failed to complete task",
			"domain_action", "complete_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return err
	}

	logger.L(ctx).Infow("Task completed successfully",
		"domain_action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID.String(),
	)

	return nil
}

// Expire 过期任务
// 将已推送状态的任务变更为已过期状态
func (l *TaskLifecycle) Expire(_ context.Context, task *AssessmentTask) error {
	// 1. 前置状态检查
	if !task.IsOpened() {
		return errors.WithCode(code.ErrInvalidArgument, "任务未处于已推送状态，无法过期")
	}

	// 2. 调用实体的包内方法（状态变更 + 事件触发）
	return task.expire(time.Now())
}

// Cancel 取消任务
// 将任务变更为已取消状态（适用于任何非终态任务）
func (l *TaskLifecycle) Cancel(_ context.Context, task *AssessmentTask) error {
	// 1. 前置状态检查
	if task.IsTerminal() {
		return nil // 幂等操作
	}

	// 2. 调用实体的包内方法（状态变更）
	task.cancel(time.Now())
	return nil
}

// Reschedule 复用既有任务，将其重置为待推送状态。
func (l *TaskLifecycle) Reschedule(ctx context.Context, task *AssessmentTask, plannedAt time.Time) error {
	taskID := task.GetID().String()
	logger.L(ctx).Infow("Rescheduling task in domain service",
		"domain_action", "reschedule_task",
		"task_id", taskID,
		"current_status", task.GetStatus().String(),
		"planned_at", plannedAt,
	)

	if plannedAt.IsZero() {
		return errors.WithCode(code.ErrInvalidArgument, "计划时间不能为空")
	}
	if task.IsCompleted() {
		return errors.WithCode(code.ErrInvalidArgument, "已完成任务不能重新调度")
	}

	if err := task.reschedule(plannedAt); err != nil {
		logger.L(ctx).Errorw("Failed to reschedule task",
			"domain_action", "reschedule_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return err
	}

	return nil
}
