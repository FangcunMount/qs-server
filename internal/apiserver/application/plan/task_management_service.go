package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	planentryport "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// taskManagementService 任务管理服务实现
// 行为者：任务管理服务
type taskManagementService struct {
	taskRepo       plan.AssessmentTaskRepository
	taskLifecycle  *plan.TaskLifecycle
	entryGenerator planentryport.Generator
	eventPublisher event.EventPublisher
	persistence    taskPersistence
	stageReader    stageport.CurrentReader
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
	enrollmentRepo plan.EnrollmentRepository,
	txRunner apptransaction.Runner,
	entryGenerator planentryport.Generator,
	eventPublisher event.EventPublisher,
) TaskManagementService {
	service := NewTaskManagementService(taskRepo, entryGenerator, eventPublisher).(*taskManagementService)
	service.persistence = taskPersistence{tasks: taskRepo, enrollments: enrollmentRepo, tx: txRunner}
	return service
}

func WithTaskHistoricalStageRecorder(target TaskManagementService, recorder stageport.Recorder) TaskManagementService {
	if concrete, ok := target.(*taskManagementService); ok {
		concrete.persistence.recorder = recorder
		if reader, readable := recorder.(stageport.CurrentReader); readable {
			concrete.stageReader = reader
		}
	}
	return target
}

// OpenTask 开放任务
func (s *taskManagementService) OpenTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	if _, historical := historicalseed.FromContext(ctx); !historical {
		return s.openTask(ctx, orgID, taskID)
	}
	if err := s.beginHistoricalTaskAttempt(ctx, stageport.StageTaskOpen, taskID, historicalTaskBusinessAt(ctx, stageport.StageTaskOpen)); err != nil {
		return nil, err
	}
	var result *TaskResult
	err := s.persistence.withinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		result, err = s.openTask(txCtx, orgID, taskID)
		return err
	})
	if err != nil {
		s.recordHistoricalTaskFailure(ctx, stageport.StageTaskOpen, taskID, historicalTaskBusinessAt(ctx, stageport.StageTaskOpen), err)
	}
	return result, err
}

func (s *taskManagementService) openTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
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
	orgScope, err := safeconv.Int64ToUint64(orgID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的机构ID: %v", err)
	}
	openedAt, err := historicalseed.OccurredAt(ctx, orgScope, historicalseed.StageTaskOpened, time.Now())
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务开放时间: %v", err)
	}
	if replayed, replayErr := s.replayHistoricalTaskStage(ctx, stageport.StageTaskOpen, task, openedAt, ""); replayErr != nil {
		return nil, replayErr
	} else if replayed {
		return toTaskResult(task), nil
	}

	// 2. 生成入口
	token, url, expireAt, err := s.entryGenerator.GenerateEntry(ctx, task)
	if err != nil {
		logger.L(ctx).Errorw("Failed to generate entry",
			"action", "open_task",
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInternalServerError, "生成任务入口失败")
	}

	// 3. 调用领域服务开放任务
	if err := s.taskLifecycle.OpenAt(ctx, task, token, url, openedAt, expireAt); err != nil {
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

// CompleteTask 完成任务
func (s *taskManagementService) CompleteTask(ctx context.Context, orgID int64, taskID string, assessmentID string) (*TaskResult, error) {
	if _, historical := historicalseed.FromContext(ctx); !historical {
		return s.completeTask(ctx, orgID, taskID, assessmentID)
	}
	if err := s.beginHistoricalTaskAttempt(ctx, stageport.StageTaskComplete, taskID, historicalTaskBusinessAt(ctx, stageport.StageTaskComplete)); err != nil {
		return nil, err
	}
	var result *TaskResult
	err := s.persistence.withinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		result, err = s.completeTask(txCtx, orgID, taskID, assessmentID)
		return err
	})
	if err != nil {
		s.recordHistoricalTaskFailure(ctx, stageport.StageTaskComplete, taskID, historicalTaskBusinessAt(ctx, stageport.StageTaskComplete), err)
	}
	return result, err
}

func (s *taskManagementService) completeTask(ctx context.Context, orgID int64, taskID string, assessmentID string) (*TaskResult, error) {
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
	orgScope, err := safeconv.Int64ToUint64(orgID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的机构ID: %v", err)
	}
	completedAt, err := historicalseed.OccurredAt(ctx, orgScope, historicalseed.StageTaskCompleted, time.Now())
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的任务完成时间: %v", err)
	}
	if replayed, replayErr := s.replayHistoricalTaskStage(ctx, stageport.StageTaskComplete, task, completedAt, assessmentIDDomain.String()); replayErr != nil {
		return nil, replayErr
	} else if replayed {
		return toTaskResult(task), nil
	}
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

type taskForUpdateReader interface {
	FindByIDForUpdate(context.Context, plan.AssessmentTaskID) (*plan.AssessmentTask, error)
}

func (s *taskManagementService) loadTaskForTransition(ctx context.Context, orgID int64, taskID, action string) (*plan.AssessmentTask, error) {
	if _, historical := historicalseed.FromContext(ctx); !historical {
		return loadTaskInOrg(ctx, s.taskRepo, orgID, taskID, action)
	}
	locking, ok := s.taskRepo.(taskForUpdateReader)
	if !ok {
		return nil, fmt.Errorf("historical task repository does not support row locking")
	}
	return loadTaskInOrgWithFinder(ctx, locking.FindByIDForUpdate, orgID, taskID, action)
}

func (s *taskManagementService) replayHistoricalTaskStage(ctx context.Context, stage string, task *plan.AssessmentTask, businessAt time.Time, assessmentID string) (bool, error) {
	if _, historical := historicalseed.FromContext(ctx); !historical {
		return false, nil
	}
	if s.stageReader == nil {
		return false, fmt.Errorf("historical task stage reader is not configured")
	}
	record, err := s.stageReader.FindCurrent(ctx, stage)
	if err != nil || record == nil {
		return false, err
	}
	want := historicalTaskStagePayload{TaskID: task.GetID().String(), EnrollmentID: task.GetEnrollmentID().String(), AssessmentID: assessmentID}
	var got historicalTaskStagePayload
	if unmarshalErr := json.Unmarshal(record.PayloadJSON, &got); unmarshalErr != nil {
		return false, fmt.Errorf("decode historical task stage %s: %w", stage, unmarshalErr)
	}
	if record.Status != "completed" || record.ResourceType != "plan_task" || record.ResourceID != want.TaskID || !record.BusinessAt.Equal(businessAt) || got != want {
		return false, fmt.Errorf("%w: task=%s stage=%s", stageport.ErrPayloadConflict, want.TaskID, stage)
	}
	switch stage {
	case stageport.StageTaskOpen:
		openedAt := task.GetOpenAt()
		if openedAt == nil || !openedAt.Equal(businessAt) || task.IsPending() {
			return false, fmt.Errorf("%w: task=%s stage=%s persisted task does not match", stageport.ErrPayloadConflict, want.TaskID, stage)
		}
	case stageport.StageTaskComplete:
		completedAt := task.GetCompletedAt()
		persistedAssessmentID := task.GetAssessmentID()
		if completedAt == nil || !completedAt.Equal(businessAt) || persistedAssessmentID == nil || persistedAssessmentID.String() != assessmentID {
			return false, fmt.Errorf("%w: task=%s stage=%s persisted task does not match", stageport.ErrPayloadConflict, want.TaskID, stage)
		}
	}
	if s.persistence.recorder != nil {
		if _, err := s.persistence.recorder.Complete(ctx, stageport.Completion{
			Stage: stage, BusinessAt: businessAt, ResourceType: "plan_task", ResourceID: want.TaskID, Payload: want,
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *taskManagementService) recordHistoricalTaskFailure(ctx context.Context, stage, taskID string, businessAt time.Time, transitionErr error) {
	recorder, ok := s.persistence.recorder.(stageport.AttemptRecorder)
	if !ok || transitionErr == nil || businessAt.IsZero() {
		return
	}
	if err := recorder.RecordFailure(ctx, stageport.Failure{Stage: stage, BusinessAt: businessAt, ResourceType: "plan_task", ResourceID: taskID, Err: transitionErr}); err != nil {
		logger.L(ctx).Errorw("Failed to record historical task attempt",
			"action", stage,
			"task_id", taskID,
			"error", err.Error(),
		)
	}
}

func (s *taskManagementService) beginHistoricalTaskAttempt(ctx context.Context, stage, taskID string, businessAt time.Time) error {
	recorder, ok := s.persistence.recorder.(stageport.AttemptRecorder)
	if !ok || businessAt.IsZero() {
		return nil
	}
	return recorder.Begin(ctx, stageport.Attempt{Stage: stage, BusinessAt: businessAt, ResourceType: "plan_task", ResourceID: taskID})
}

func historicalTaskBusinessAt(ctx context.Context, stage string) time.Time {
	historical, ok := historicalseed.FromContext(ctx)
	if !ok {
		return time.Time{}
	}
	switch stage {
	case stageport.StageTaskOpen:
		if historical.Timeline.TaskOpenedAt != nil {
			return *historical.Timeline.TaskOpenedAt
		}
	case stageport.StageTaskComplete:
		if historical.Timeline.TaskCompletedAt != nil {
			return *historical.Timeline.TaskCompletedAt
		}
	}
	return time.Time{}
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
