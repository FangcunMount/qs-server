package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planentryport "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// taskManagementService 任务管理服务实现
// 行为者：任务管理服务
type taskManagementService struct {
	taskRepo       plan.AssessmentTaskRepository
	planRepo       plan.AssessmentPlanRepository
	enrollmentRepo plan.EnrollmentRepository
	taskLifecycle  *plan.TaskLifecycle
	entryGenerator planentryport.Generator
	eventPublisher event.EventPublisher
	persistence    taskPersistence
}

// NewTaskManagementService 创建任务管理服务
func NewTaskManagementService(
	taskRepo plan.AssessmentTaskRepository,
	entryGenerator planentryport.Generator,
	eventPublisher event.EventPublisher,
) TaskManagementService {
	taskLifecycle := plan.NewTaskLifecycle()
	return &taskManagementService{
		taskRepo:       taskRepo,
		taskLifecycle:  taskLifecycle,
		entryGenerator: entryGenerator,
		eventPublisher: eventPublisher,
		persistence:    taskPersistence{tasks: taskRepo},
	}
}

func NewTaskManagementServiceWithEnrollment(
	taskRepo plan.AssessmentTaskRepository,
	planRepo plan.AssessmentPlanRepository,
	enrollmentRepo plan.EnrollmentRepository,
	txRunner apptransaction.Runner,
	entryGenerator planentryport.Generator,
	eventPublisher event.EventPublisher,
) TaskManagementService {
	service := NewTaskManagementService(taskRepo, entryGenerator, eventPublisher).(*taskManagementService)
	service.planRepo = planRepo
	service.enrollmentRepo = enrollmentRepo
	service.persistence = taskPersistence{tasks: taskRepo, enrollments: enrollmentRepo, tx: txRunner}
	return service
}

// OpenTask 开放任务
func (s *taskManagementService) OpenTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Opening task",
		"action", "open_task",
		"org_id", orgID,
		"task_id", taskID,
	)

	// 1. 查询并校验任务
	task, err := s.loadTaskForTransition(ctx, orgID, taskID, "open_task")
	if err != nil {
		return nil, err
	}
	openedAt := time.Now()
	if err := s.validateOpenAdmission(ctx, task, openedAt); err != nil {
		return nil, err
	}

	// 2. 生成入口
	token, url, err := s.entryGenerator.GenerateEntry(ctx, task)
	if err != nil {
		logger.L(ctx).Errorw("Failed to generate entry",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInternalServerError, "生成任务入口失败")
	}

	// 3. 调用领域服务开放任务
	if err := s.taskLifecycle.OpenAt(ctx, task, token, url, openedAt, plan.TaskEntryExpiresAt(openedAt)); err != nil {
		logger.L(ctx).Errorw("Failed to open task",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 4. 持久化
	if err := s.persistence.save(ctx, task, false); err != nil {
		logger.L(ctx).Errorw("Failed to save opened task",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event",
			"action", "open_task",
			"task_id", taskID,
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})

	logger.L(ctx).Infow("Task opened successfully",
		"action", "open_task",
		"task_id", taskID,
	)

	return toTaskResult(task), nil
}

func (s *taskManagementService) validateOpenAdmission(ctx context.Context, task *plan.AssessmentTask, actionAt time.Time) error {
	if actionAt.Before(task.GetPlannedAt()) || !actionAt.Before(plan.TaskOpenWindowEndsAt(task.GetPlannedAt())) {
		return errors.WithCode(errorCode.ErrInvalidArgument, "任务不在开放窗口内")
	}
	if s.planRepo != nil {
		parent, err := s.planRepo.FindByID(ctx, task.GetPlanID())
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询任务计划失败")
		}
		if parent == nil || parent.GetOrgID() != task.GetOrgID() || !parent.IsActive() {
			return errors.WithCode(errorCode.ErrInvalidArgument, "任务计划未处于 active 状态")
		}
	}
	if s.enrollmentRepo != nil {
		enrollment, err := s.enrollmentRepo.FindByID(ctx, task.GetEnrollmentID())
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询任务参与轮次失败")
		}
		if enrollment == nil || enrollment.OrgID() != task.GetOrgID() || !enrollment.IsActive() {
			return errors.WithCode(errorCode.ErrInvalidArgument, "任务参与轮次未处于 active 状态")
		}
	}
	return nil
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
	task, err := s.loadTaskForTransition(ctx, orgID, taskID, "complete_task")
	if err != nil {
		return nil, err
	}

	// 3. 调用领域服务完成任务
	completedAt := time.Now()
	if err := s.taskLifecycle.CompleteAt(ctx, task, assessmentIDDomain, completedAt); err != nil {
		logger.L(ctx).Errorw("Failed to complete task",
			"action", "complete_task",
			"task_id", taskID,
			"assessment_id", assessmentID,
			"error", err.Error(),
		)
		return nil, err
	}

	// 4. 持久化
	if err := s.persistence.save(ctx, task, true); err != nil {
		logger.L(ctx).Errorw("Failed to save completed task",
			"action", "complete_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 5. 发布领域事件
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event",
			"action", "complete_task",
			"task_id", taskID,
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})

	logger.L(ctx).Infow("Task completed successfully",
		"action", "complete_task",
		"task_id", taskID,
		"assessment_id", assessmentID,
	)

	return toTaskResult(task), nil
}

func (s *taskManagementService) loadTaskForTransition(ctx context.Context, orgID int64, taskID, action string) (*plan.AssessmentTask, error) {
	return loadTaskInOrg(ctx, s.taskRepo, orgID, taskID, action)
}

// ExpireTask 过期任务
func (s *taskManagementService) ExpireTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	logger.L(ctx).Infow("Expiring task",
		"action", "expire_task",
		"org_id", orgID,
		"task_id", taskID,
	)

	// 1. 查询并校验任务
	task, err := loadTaskInOrg(ctx, s.taskRepo, orgID, taskID, "expire_task")
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
	if err := s.persistence.save(ctx, task, true); err != nil {
		logger.L(ctx).Errorw("Failed to save expired task",
			"action", "expire_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 4. 发布领域事件
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event",
			"action", "expire_task",
			"task_id", taskID,
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})

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
	task, err := loadTaskInOrg(ctx, s.taskRepo, orgID, taskID, "cancel_task")
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
	if err := s.persistence.save(ctx, task, true); err != nil {
		logger.L(ctx).Errorw("Failed to save canceled task",
			"action", "cancel_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
	}

	// 4. 发布领域事件
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event",
			"action", "cancel_task",
			"task_id", taskID,
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})

	logger.L(ctx).Infow("Task canceled successfully",
		"action", "cancel_task",
		"task_id", taskID,
	)

	return nil
}
