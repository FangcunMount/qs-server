package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// taskManagementService 任务管理服务实现
// 行为者：任务管理服务
type taskManagementService struct {
	taskRepo       plan.AssessmentTaskRepository
	taskLifecycle  *plan.TaskLifecycle
	eventPublisher event.EventPublisher
}

// NewTaskManagementService 创建任务管理服务
func NewTaskManagementService(
	taskRepo plan.AssessmentTaskRepository,
	eventPublisher event.EventPublisher,
) TaskManagementService {
	return &taskManagementService{
		taskRepo:       taskRepo,
		taskLifecycle:  plan.NewTaskLifecycle(),
		eventPublisher: eventPublisher,
	}
}

// OpenTask 开放任务
func (s *taskManagementService) OpenTask(ctx context.Context, taskID string, dto OpenTaskDTO) (*TaskResult, error) {
	logger.L(ctx).Infow("Opening task",
		"action", "open_task",
		"task_id", taskID,
		"expire_at", dto.ExpireAt,
	)

	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	expireAt, err := parseTime(dto.ExpireAt)
	if err != nil {
		logger.L(ctx).Errorw("Invalid expire time",
			"action", "open_task",
			"task_id", taskID,
			"expire_at", dto.ExpireAt,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的过期时间: %v", err)
	}

	// 2. 查询任务
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	// 3. 调用领域服务开放任务
	if err := s.taskLifecycle.Open(ctx, task, dto.EntryToken, dto.EntryURL, expireAt); err != nil {
		logger.L(ctx).Errorw("Failed to open task",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 4. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save opened task",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	events := task.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "open_task",
				"task_id", taskID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	task.ClearEvents()

	logger.L(ctx).Infow("Task opened successfully",
		"action", "open_task",
		"task_id", taskID,
	)

	return toTaskResult(task), nil
}

// CompleteTask 完成任务
func (s *taskManagementService) CompleteTask(ctx context.Context, taskID string, assessmentID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Completing task",
		"action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID,
	)

	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", "complete_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	assessmentIDDomain, err := assessment.ParseID(assessmentID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid assessment ID",
			"action", "complete_task",
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的测评ID: %v", err)
	}

	// 2. 查询任务
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", "complete_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	// 3. 调用领域服务完成任务
	if err := s.taskLifecycle.Complete(ctx, task, assessmentIDDomain); err != nil {
		logger.L(ctx).Errorw("Failed to complete task",
			"action", "complete_task",
			"task_id", taskID,
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 4. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save completed task",
			"action", "complete_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	events := task.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "complete_task",
				"task_id", taskID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	task.ClearEvents()

	logger.L(ctx).Infow("Task completed successfully",
		"action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID,
	)

	return toTaskResult(task), nil
}

// ExpireTask 过期任务
func (s *taskManagementService) ExpireTask(ctx context.Context, taskID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Expiring task",
		"action", "expire_task",
		"task_id", taskID,
	)

	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	// 2. 查询任务
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	// 3. 调用领域服务过期任务
	if err := s.taskLifecycle.Expire(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to expire task",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 4. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save expired task",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	events := task.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "expire_task",
				"task_id", taskID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	task.ClearEvents()

	logger.L(ctx).Infow("Task expired successfully",
		"action", "expire_task",
		"task_id", taskID,
	)

	return toTaskResult(task), nil
}

// CancelTask 取消任务
func (s *taskManagementService) CancelTask(ctx context.Context, taskID string) error {
	logger.L(ctx).Infow("Canceling task",
		"action", "cancel_task",
		"task_id", taskID,
	)

	// 1. 转换参数
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	// 2. 查询任务
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	// 3. 调用领域服务取消任务
	if err := s.taskLifecycle.Cancel(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to cancel task",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return err
	}

	// 4. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save canceled task",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	events := task.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "cancel_task",
				"task_id", taskID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	task.ClearEvents()

	logger.L(ctx).Infow("Task canceled successfully",
		"action", "cancel_task",
		"task_id", taskID,
	)

	return nil
}
