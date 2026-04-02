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
	planRepo       plan.AssessmentPlanRepository
	taskLifecycle  *plan.TaskLifecycle
	planLifecycle  *plan.PlanLifecycle
	eventPublisher event.EventPublisher
}

// NewTaskManagementService 创建任务管理服务
func NewTaskManagementService(
	taskRepo plan.AssessmentTaskRepository,
	planRepo plan.AssessmentPlanRepository,
	eventPublisher event.EventPublisher,
) TaskManagementService {
	taskGenerator := plan.NewTaskGenerator()
	taskLifecycle := plan.NewTaskLifecycle()
	return &taskManagementService{
		taskRepo:       taskRepo,
		planRepo:       planRepo,
		taskLifecycle:  taskLifecycle,
		planLifecycle:  plan.NewPlanLifecycle(taskRepo, taskGenerator, taskLifecycle),
		eventPublisher: eventPublisher,
	}
}

// OpenTask 开放任务
func (s *taskManagementService) OpenTask(ctx context.Context, orgID int64, taskID string, dto OpenTaskDTO) (*TaskResult, error) {
	logger.L(ctx).Infow("Opening task",
		"action", "open_task",
		"org_id", orgID,
		"task_id", taskID,
		"expire_at", dto.ExpireAt,
	)

	// 1. 解析过期时间
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

	// 2. 查询并校验任务
	task, err := s.loadTaskInOrg(ctx, orgID, taskID, "open_task")
	if err != nil {
		return nil, err
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
func (s *taskManagementService) CompleteTask(ctx context.Context, orgID int64, taskID string, assessmentID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Completing task",
		"action", "complete_task",
		"org_id", orgID,
		"task_id", taskID,
		"assessment_id", assessmentID,
	)

	// 1. 转换参数
	assessmentIDDomain, err := assessment.ParseID(assessmentID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid assessment ID",
			"action", "complete_task",
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的测评ID: %v", err)
	}

	// 2. 查询并校验任务
	task, err := s.loadTaskInOrg(ctx, orgID, taskID, "complete_task")
	if err != nil {
		return nil, err
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

	if err := s.finishPlanIfDone(ctx, task.GetPlanID()); err != nil {
		logger.L(ctx).Warnw("Failed to finalize plan after task completion",
			"action", "complete_task",
			"task_id", taskID,
			"plan_id", task.GetPlanID().String(),
			"error", err.Error(),
		)
	}

	logger.L(ctx).Infow("Task completed successfully",
		"action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID,
	)

	return toTaskResult(task), nil
}

// ExpireTask 过期任务
func (s *taskManagementService) ExpireTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Expiring task",
		"action", "expire_task",
		"org_id", orgID,
		"task_id", taskID,
	)

	// 1. 查询并校验任务
	task, err := s.loadTaskInOrg(ctx, orgID, taskID, "expire_task")
	if err != nil {
		return nil, err
	}

	// 2. 调用领域服务过期任务
	if err := s.taskLifecycle.Expire(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to expire task",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 3. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save expired task",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 4. 发布领域事件
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

	if err := s.finishPlanIfDone(ctx, task.GetPlanID()); err != nil {
		logger.L(ctx).Warnw("Failed to finalize plan after task expiration",
			"action", "expire_task",
			"task_id", taskID,
			"plan_id", task.GetPlanID().String(),
			"error", err.Error(),
		)
	}

	logger.L(ctx).Infow("Task expired successfully",
		"action", "expire_task",
		"task_id", taskID,
	)

	return toTaskResult(task), nil
}

// CancelTask 取消任务
func (s *taskManagementService) CancelTask(ctx context.Context, orgID int64, taskID string) error {
	logger.L(ctx).Infow("Canceling task",
		"action", "cancel_task",
		"org_id", orgID,
		"task_id", taskID,
	)

	// 1. 查询并校验任务
	task, err := s.loadTaskInOrg(ctx, orgID, taskID, "cancel_task")
	if err != nil {
		return err
	}

	// 2. 调用领域服务取消任务
	if err := s.taskLifecycle.Cancel(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to cancel task",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return err
	}

	// 3. 持久化
	if err := s.taskRepo.Save(ctx, task); err != nil {
		logger.L(ctx).Errorw("Failed to save canceled task",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 4. 发布领域事件
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

	if err := s.finishPlanIfDone(ctx, task.GetPlanID()); err != nil {
		logger.L(ctx).Warnw("Failed to finalize plan after task cancellation",
			"action", "cancel_task",
			"task_id", taskID,
			"plan_id", task.GetPlanID().String(),
			"error", err.Error(),
		)
	}

	logger.L(ctx).Infow("Task canceled successfully",
		"action", "cancel_task",
		"task_id", taskID,
	)

	return nil
}

func (s *taskManagementService) loadTaskInOrg(ctx context.Context, orgID int64, taskID string, action string) (*plan.AssessmentTask, error) {
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", action,
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", action,
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	if task.GetOrgID() != orgID {
		logger.L(ctx).Warnw("Task access denied due to org scope mismatch",
			"action", action,
			"task_id", taskID,
			"request_org_id", orgID,
			"resource_org_id", task.GetOrgID(),
		)
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "任务不属于当前机构")
	}

	return task, nil
}

func (s *taskManagementService) finishPlanIfDone(ctx context.Context, planID plan.AssessmentPlanID) error {
	return finalizePlanIfDone(
		ctx,
		"finish_plan_after_task_transition",
		s.planRepo,
		s.planLifecycle,
		s.eventPublisher,
		planID,
	)
}
