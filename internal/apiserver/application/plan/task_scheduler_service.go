package plan

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planentryport "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// taskSchedulerService 任务调度服务实现
// 行为者：任务调度服务
type taskSchedulerService struct {
	taskRepo       plan.AssessmentTaskRepository
	planRepo       plan.AssessmentPlanRepository
	enrollmentRepo plan.EnrollmentRepository
	taskLifecycle  *plan.TaskLifecycle
	entryGenerator planentryport.Generator // 入口生成器（由基础设施层实现）
	eventPublisher event.EventPublisher
	persistence    taskPersistence
}

// NewTaskSchedulerService 创建任务调度服务
func NewTaskSchedulerService(
	taskRepo plan.AssessmentTaskRepository,
	planRepo plan.AssessmentPlanRepository,
	entryGenerator planentryport.Generator,
	eventPublisher event.EventPublisher,
) TaskSchedulerService {
	taskLifecycle := plan.NewTaskLifecycle()
	return &taskSchedulerService{
		taskRepo:       taskRepo,
		planRepo:       planRepo,
		taskLifecycle:  taskLifecycle,
		entryGenerator: entryGenerator,
		eventPublisher: eventPublisher,
		persistence:    taskPersistence{tasks: taskRepo},
	}
}

func NewTaskSchedulerServiceWithEnrollment(
	taskRepo plan.AssessmentTaskRepository,
	planRepo plan.AssessmentPlanRepository,
	enrollmentRepo plan.EnrollmentRepository,
	txRunner apptransaction.Runner,
	entryGenerator planentryport.Generator,
	eventPublisher event.EventPublisher,
) TaskSchedulerService {
	service := NewTaskSchedulerService(taskRepo, planRepo, entryGenerator, eventPublisher).(*taskSchedulerService)
	service.enrollmentRepo = enrollmentRepo
	service.persistence = taskPersistence{tasks: taskRepo, enrollments: enrollmentRepo, tx: txRunner}
	return service
}

// SchedulePendingTasks 调度待推送的任务
func (s *taskSchedulerService) SchedulePendingTasks(ctx context.Context, orgID int64, before string) ([]*TaskResult, error) {
	scope := taskSchedulerScopeFromContext(ctx)
	logger.L(ctx).Infow("Scheduling pending tasks",
		"action", "schedule_pending_tasks",
		"org_id", orgID,
		"before", before,
		"scope_plan_id", scopePlanID(scope),
		"scope_testee_count", scopeTesteeCount(scope),
	)
	if orgID <= 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的机构ID")
	}

	// 1. 解析时间参数
	beforeTime, err := parseTime(before)
	if err != nil {
		logger.L(ctx).Errorw("Invalid time format",
			"action", "schedule_pending_tasks",
			"before", before,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的时间格式: %v", err)
	}

	planCache := make(map[string]*plan.AssessmentPlan)
	enrollmentCache := make(map[string]*plan.Enrollment)

	// 2. First close opened entries whose hard validity already elapsed.
	expiredCount, expireFailedCount, openedOverdueCount, oldestOpenedAge, err := s.expireOverdueTasks(ctx, orgID, beforeTime, planCache, enrollmentCache)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询入口已失效任务失败")
	}

	// 3. 查询仍处于开放窗口内的待推送任务
	tasks, err := s.findPendingTasks(ctx, orgID, beforeTime)
	if err != nil {
		logger.L(ctx).Errorw("Failed to find pending tasks",
			"action", "schedule_pending_tasks",
			"org_id", orgID,
			"before", before,
			"scope_plan_id", scopePlanID(scope),
			"scope_testee_count", scopeTesteeCount(scope),
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询待推送任务失败")
	}

	logger.L(ctx).Infow("Found pending tasks",
		"action", "schedule_pending_tasks",
		"org_id", orgID,
		"before", before,
		"scope_plan_id", scopePlanID(scope),
		"scope_testee_count", scopeTesteeCount(scope),
		"pending_tasks_count", len(tasks),
	)

	// 4. 为每个任务生成入口并开放
	var openedTasks []*plan.AssessmentTask
	failedCount := 0
	inactivePlanCanceledCount := 0
	for _, task := range tasks {
		parentPlan, err := s.loadPlanForTask(ctx, planCache, task)
		if err != nil {
			logger.L(ctx).Errorw("Failed to load parent plan for task scheduling",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"plan_id", task.GetPlanID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}
		if parentPlan != nil && !parentPlan.IsActive() {
			if err := s.cancelTaskForInactivePlan(ctx, task, parentPlan); err != nil {
				logger.L(ctx).Errorw("Failed to cancel pending task for inactive plan",
					"action", "schedule_pending_tasks",
					"task_id", task.GetID().String(),
					"plan_id", task.GetPlanID().String(),
					"plan_status", parentPlan.GetStatus().String(),
					"error", err.Error(),
				)
				failedCount++
				continue
			}
			inactivePlanCanceledCount++
			continue
		}
		active, err := s.enrollmentIsActive(ctx, enrollmentCache, task)
		if err != nil {
			logger.L(ctx).Errorw("Failed to load enrollment for task scheduling", "task_id", task.GetID().String(), "error", err.Error())
			failedCount++
			continue
		}
		if !active {
			if err := s.cancelTaskForInactiveEnrollment(ctx, task); err != nil {
				failedCount++
			}
			continue
		}

		// 生成入口
		token, url, err := s.entryGenerator.GenerateEntry(ctx, task)
		if err != nil {
			logger.L(ctx).Errorw("Failed to generate entry",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		// 开放任务
		openedAt := time.Now()
		if err := s.taskLifecycle.OpenAt(ctx, task, token, url, openedAt); err != nil {
			logger.L(ctx).Errorw("Failed to open task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		// 持久化任务
		if err := s.persistence.save(ctx, task, false); err != nil {
			logger.L(ctx).Errorw("Failed to save opened task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		})

		openedTasks = append(openedTasks, task)
	}

	missedTasks, err := s.findMissedOpenWindowTasks(ctx, orgID, beforeTime)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询错过开放窗口任务失败")
	}
	missedExpiredCount := 0
	missedExpireFailedCount := 0
	if TaskSchedulerMissedExpirationEnabled(ctx) {
		missedExpiredCount, missedExpireFailedCount = s.expireMissedOpenWindowTasks(ctx, orgID, missedTasks, planCache, enrollmentCache)
	}
	missedBacklogCount, backlogErr := s.missedBacklogCount(ctx, orgID, beforeTime, missedTasks)
	if backlogErr != nil {
		return nil, errors.WrapC(backlogErr, errorCode.ErrDatabase, "检查错过开放窗口积压失败")
	}
	oldestPendingAge := oldestTaskAgeSeconds(beforeTime, tasks, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() })
	if missedAge := oldestTaskAgeSeconds(beforeTime, missedTasks, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() }); missedAge > oldestPendingAge {
		oldestPendingAge = missedAge
	}
	CollectTaskScheduleStats(ctx, TaskScheduleStats{
		PendingCount:            len(tasks),
		OpenedCount:             len(openedTasks),
		FailedCount:             failedCount,
		OpenedOverdueCount:      openedOverdueCount,
		ExpiredCount:            expiredCount,
		ExpireFailedCount:       expireFailedCount,
		MissedCandidateCount:    len(missedTasks),
		MissedExpiredCount:      missedExpiredCount,
		MissedExpireFailedCount: missedExpireFailedCount,
		MissedBacklogCount:      missedBacklogCount,
		OldestPendingAgeSeconds: oldestPendingAge,
		OldestOpenedAgeSeconds:  oldestOpenedAge,
	})

	logger.L(ctx).Infow("Tasks scheduled",
		"action", "schedule_pending_tasks",
		"org_id", orgID,
		"before", before,
		"scope_plan_id", scopePlanID(scope),
		"scope_testee_count", scopeTesteeCount(scope),
		"total_pending", len(tasks),
		"opened_count", len(openedTasks),
		"failed_count", failedCount,
		"inactive_plan_canceled_count", inactivePlanCanceledCount,
		"expired_count", expiredCount,
		"expire_failed_count", expireFailedCount,
		"missed_expired_count", missedExpiredCount,
		"missed_expire_failed_count", missedExpireFailedCount,
		"missed_candidate_count", len(missedTasks),
		"missed_backlog_count", missedBacklogCount,
		"opened_overdue_count", openedOverdueCount,
		"oldest_pending_age_seconds", oldestPendingAge,
		"oldest_opened_overdue_age_seconds", oldestOpenedAge,
	)

	if failureCount := failedCount + expireFailedCount + missedExpireFailedCount; failureCount > 0 {
		return toTaskResults(openedTasks), errors.WithCode(errorCode.ErrInternalServerError, "任务调度存在 %d 个失败项", failureCount)
	}
	return toTaskResults(openedTasks), nil
}

func (s *taskSchedulerService) findPendingTasks(ctx context.Context, orgID int64, before time.Time) ([]*plan.AssessmentTask, error) {
	scope := taskSchedulerScopeFromContext(ctx)
	if scope == nil || (strings.TrimSpace(scope.PlanID) == "" && len(scope.TesteeIDs) == 0) {
		if scanner, ok := s.taskRepo.(plan.AssessmentTaskSchedulerRepository); ok {
			lowerBound := before.Add(-plan.TaskOpenWindow)
			if configured, exists := TaskSchedulerPlannedAtLowerBoundFromContext(ctx); exists && configured.After(lowerBound) {
				lowerBound = configured
			}
			return scanOpenEligibleTasks(ctx, scanner, orgID, lowerBound, before)
		}
		tasks, err := s.taskRepo.FindPendingTasks(ctx, orgID, before)
		if err != nil {
			return nil, err
		}
		return filterSchedulablePendingTasks(ctx, tasks, orgID, before), nil
	}
	planID := strings.TrimSpace(scope.PlanID)
	if planID == "" {
		tasks, err := s.taskRepo.FindPendingTasks(ctx, orgID, before)
		if err != nil {
			return nil, err
		}
		return filterSchedulablePendingTasks(ctx, tasks, orgID, before), nil
	}

	parsedPlanID, err := plan.ParseAssessmentPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID")
	}

	var scopedTesteeIDs []testee.ID
	if len(scope.TesteeIDs) > 0 {
		scopedTesteeIDs, err = parseScheduleScopeTesteeIDs(scope.TesteeIDs)
		if err != nil {
			return nil, err
		}
	}
	if scanner, ok := s.taskRepo.(plan.AssessmentTaskScopedSchedulerRepository); ok {
		lowerBound := before.Add(-plan.TaskOpenWindow)
		if configured, exists := TaskSchedulerPlannedAtLowerBoundFromContext(ctx); exists && configured.After(lowerBound) {
			lowerBound = configured
		}
		return scanScopedOpenEligibleTasks(ctx, scanner, orgID, parsedPlanID, scopedTesteeIDs, lowerBound, before)
	}

	var tasks []*plan.AssessmentTask
	if len(scopedTesteeIDs) > 0 {
		tasks, err = s.taskRepo.FindByPlanIDAndTesteeIDs(ctx, parsedPlanID, scopedTesteeIDs)
	} else {
		tasks, err = s.taskRepo.FindByPlanID(ctx, parsedPlanID)
	}
	if err != nil {
		return nil, err
	}

	return filterSchedulablePendingTasks(ctx, tasks, orgID, before), nil
}

func filterSchedulablePendingTasks(ctx context.Context, tasks []*plan.AssessmentTask, orgID int64, before time.Time) []*plan.AssessmentTask {
	filtered := make([]*plan.AssessmentTask, 0, len(tasks))
	lowerBound := before.Add(-plan.TaskOpenWindow)
	if configured, exists := TaskSchedulerPlannedAtLowerBoundFromContext(ctx); exists && configured.After(lowerBound) {
		lowerBound = configured
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.GetOrgID() != orgID || !task.IsPending() {
			continue
		}
		if !task.GetPlannedAt().After(lowerBound) {
			continue
		}
		if task.GetPlannedAt().After(before) {
			continue
		}
		filtered = append(filtered, task)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].GetPlannedAt().Equal(filtered[j].GetPlannedAt()) {
			return filtered[i].GetID().Uint64() < filtered[j].GetID().Uint64()
		}
		return filtered[i].GetPlannedAt().Before(filtered[j].GetPlannedAt())
	})
	return filtered
}

func parseScheduleScopeTesteeIDs(rawIDs []string) ([]testee.ID, error) {
	parsed := make([]testee.ID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" {
			continue
		}
		id, err := meta.ParseID(rawID)
		if err != nil {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID")
		}
		if id.IsZero() {
			continue
		}
		parsed = append(parsed, id)
	}
	return parsed, nil
}

func scopePlanID(scope *TaskSchedulerScope) string {
	if scope == nil {
		return ""
	}
	return scope.PlanID
}

func scopeTesteeCount(scope *TaskSchedulerScope) int {
	if scope == nil {
		return 0
	}
	return len(scope.TesteeIDs)
}

func (s *taskSchedulerService) expireOverdueTasks(ctx context.Context, orgID int64, actionAt time.Time, planCache map[string]*plan.AssessmentPlan, enrollmentCache map[string]*plan.Enrollment) (int, int, int, int64, error) {
	var tasks []*plan.AssessmentTask
	var err error
	if scanner, ok := s.taskRepo.(plan.AssessmentTaskSchedulerRepository); ok {
		tasks, err = scanEntryExpiredTasks(ctx, scanner, orgID, actionAt)
	} else {
		tasks, err = s.taskRepo.FindExpiredTasks(ctx)
	}
	if err != nil {
		logger.L(ctx).Errorw("Failed to find expired tasks",
			"action", "schedule_pending_tasks",
			"error", err.Error(),
		)
		return 0, 0, 0, 0, err
	}
	oldestAge := oldestTaskAgeSeconds(actionAt, tasks, func(task *plan.AssessmentTask) time.Time {
		if task.GetExpireAt() == nil {
			return time.Time{}
		}
		return *task.GetExpireAt()
	})

	expiredCount := 0
	failedCount := 0
	for _, task := range tasks {
		if task.GetOrgID() != orgID {
			continue
		}
		parentPlan, err := s.loadPlanForTask(ctx, planCache, task)
		if err != nil {
			logger.L(ctx).Errorw("Failed to load parent plan for expiring task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"plan_id", task.GetPlanID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}
		if parentPlan != nil && !parentPlan.IsActive() {
			if err := s.cancelTaskForInactivePlan(ctx, task, parentPlan); err != nil {
				logger.L(ctx).Errorw("Failed to cancel opened task for inactive plan",
					"action", "schedule_pending_tasks",
					"task_id", task.GetID().String(),
					"plan_id", task.GetPlanID().String(),
					"plan_status", parentPlan.GetStatus().String(),
					"error", err.Error(),
				)
				failedCount++
			}
			continue
		}
		active, err := s.enrollmentIsActive(ctx, enrollmentCache, task)
		if err != nil {
			failedCount++
			continue
		}
		if !active {
			if err := s.cancelTaskForInactiveEnrollment(ctx, task); err != nil {
				failedCount++
			}
			continue
		}
		if err := s.taskLifecycle.ExpireAt(task, plan.TaskExpirationReasonEntryTimeout, actionAt); err != nil {
			logger.L(ctx).Errorw("Failed to expire task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		if err := s.persistence.save(ctx, task, true); err != nil {
			logger.L(ctx).Errorw("Failed to save expired task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
			logger.L(ctx).Errorw("Failed to publish expired task event",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		})

		expiredCount++
	}

	return expiredCount, failedCount, len(tasks), oldestAge, nil
}

func scanOpenEligibleTasks(ctx context.Context, scanner plan.AssessmentTaskSchedulerRepository, orgID int64, after, through time.Time) ([]*plan.AssessmentTask, error) {
	batchSize, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
	return scanTaskPages(func(cursorAt time.Time, cursorID uint64, limit int) ([]*plan.AssessmentTask, error) {
		return scanner.FindOpenEligibleTaskPage(ctx, orgID, after, through, cursorAt, cursorID, limit)
	}, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() }, batchSize, maxTasksPerTick)
}

func scanScopedOpenEligibleTasks(ctx context.Context, scanner plan.AssessmentTaskScopedSchedulerRepository, orgID int64, planID plan.AssessmentPlanID, testeeIDs []testee.ID, after, through time.Time) ([]*plan.AssessmentTask, error) {
	batchSize, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
	return scanTaskPages(func(cursorAt time.Time, cursorID uint64, limit int) ([]*plan.AssessmentTask, error) {
		return scanner.FindScopedOpenEligibleTaskPage(ctx, orgID, planID, testeeIDs, after, through, cursorAt, cursorID, limit)
	}, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() }, batchSize, maxTasksPerTick)
}

func scanScopedMissedTasks(ctx context.Context, scanner plan.AssessmentTaskScopedSchedulerRepository, orgID int64, planID plan.AssessmentPlanID, testeeIDs []testee.ID, through time.Time) ([]*plan.AssessmentTask, error) {
	batchSize, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
	return scanTaskPages(func(cursorAt time.Time, cursorID uint64, limit int) ([]*plan.AssessmentTask, error) {
		return scanner.FindScopedMissedPendingTaskPage(ctx, orgID, planID, testeeIDs, through, cursorAt, cursorID, limit)
	}, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() }, batchSize, maxTasksPerTick)
}

func scanEntryExpiredTasks(ctx context.Context, scanner plan.AssessmentTaskSchedulerRepository, orgID int64, through time.Time) ([]*plan.AssessmentTask, error) {
	batchSize, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
	return scanTaskPages(func(cursorAt time.Time, cursorID uint64, limit int) ([]*plan.AssessmentTask, error) {
		return scanner.FindEntryExpiredTaskPage(ctx, orgID, through, cursorAt, cursorID, limit)
	}, func(task *plan.AssessmentTask) time.Time {
		if task.GetExpireAt() == nil {
			return time.Time{}
		}
		return *task.GetExpireAt()
	}, batchSize, maxTasksPerTick)
}

func scanMissedTasks(ctx context.Context, scanner plan.AssessmentTaskSchedulerRepository, orgID int64, through time.Time) ([]*plan.AssessmentTask, error) {
	batchSize, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
	return scanTaskPages(func(cursorAt time.Time, cursorID uint64, limit int) ([]*plan.AssessmentTask, error) {
		return scanner.FindMissedPendingTaskPage(ctx, orgID, through, cursorAt, cursorID, limit)
	}, func(task *plan.AssessmentTask) time.Time { return task.GetPlannedAt() }, batchSize, maxTasksPerTick)
}

func scanTaskPages(fetch func(time.Time, uint64, int) ([]*plan.AssessmentTask, error), cursorTime func(*plan.AssessmentTask) time.Time, batchSize, maxTasksPerTick int) ([]*plan.AssessmentTask, error) {
	result := make([]*plan.AssessmentTask, 0, batchSize)
	var cursorAt time.Time
	var cursorID uint64
	for len(result) < maxTasksPerTick {
		limit := batchSize
		if remaining := maxTasksPerTick - len(result); remaining < limit {
			limit = remaining
		}
		page, err := fetch(cursorAt, cursorID, limit)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		result = append(result, page...)
		last := page[len(page)-1]
		cursorAt, cursorID = cursorTime(last), last.GetID().Uint64()
		if len(page) < limit {
			break
		}
	}
	return result, nil
}

func (s *taskSchedulerService) findMissedOpenWindowTasks(ctx context.Context, orgID int64, actionAt time.Time) ([]*plan.AssessmentTask, error) {
	missedThrough := actionAt.Add(-plan.TaskOpenWindow)
	var tasks []*plan.AssessmentTask
	var err error
	scope := taskSchedulerScopeFromContext(ctx)
	if scope != nil && strings.TrimSpace(scope.PlanID) != "" {
		parsedPlanID, parseErr := plan.ParseAssessmentPlanID(strings.TrimSpace(scope.PlanID))
		if parseErr != nil {
			return nil, parseErr
		}
		var ids []testee.ID
		if len(scope.TesteeIDs) > 0 {
			ids, parseErr = parseScheduleScopeTesteeIDs(scope.TesteeIDs)
			if parseErr != nil {
				return nil, parseErr
			}
		}
		if scanner, ok := s.taskRepo.(plan.AssessmentTaskScopedSchedulerRepository); ok {
			tasks, err = scanScopedMissedTasks(ctx, scanner, orgID, parsedPlanID, ids, missedThrough)
		} else if len(ids) > 0 {
			tasks, err = s.taskRepo.FindByPlanIDAndTesteeIDs(ctx, parsedPlanID, ids)
		} else {
			tasks, err = s.taskRepo.FindByPlanID(ctx, parsedPlanID)
		}
	} else if scanner, ok := s.taskRepo.(plan.AssessmentTaskSchedulerRepository); ok {
		tasks, err = scanMissedTasks(ctx, scanner, orgID, missedThrough)
	} else {
		tasks, err = s.taskRepo.FindPendingTasks(ctx, orgID, missedThrough)
		_, maxTasksPerTick := taskSchedulerLimitsFromContext(ctx)
		if len(tasks) > maxTasksPerTick {
			tasks = tasks[:maxTasksPerTick]
		}
	}
	if err != nil {
		logger.L(ctx).Errorw("Failed to find tasks that missed the opening window", "org_id", orgID, "error", err.Error())
		return nil, err
	}
	filtered := make([]*plan.AssessmentTask, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.GetOrgID() != orgID || !task.IsPending() || task.GetPlannedAt().After(missedThrough) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

func (s *taskSchedulerService) expireMissedOpenWindowTasks(ctx context.Context, orgID int64, tasks []*plan.AssessmentTask, planCache map[string]*plan.AssessmentPlan, enrollmentCache map[string]*plan.Enrollment) (int, int) {
	expiredCount := 0
	failedCount := 0
	for _, task := range tasks {
		if task == nil || task.GetOrgID() != orgID || !task.IsPending() {
			continue
		}
		parentPlan, err := s.loadPlanForTask(ctx, planCache, task)
		if err != nil {
			failedCount++
			continue
		}
		if parentPlan != nil && !parentPlan.IsActive() {
			if err := s.cancelTaskForInactivePlan(ctx, task, parentPlan); err != nil {
				failedCount++
			}
			continue
		}
		active, err := s.enrollmentIsActive(ctx, enrollmentCache, task)
		if err != nil {
			failedCount++
			continue
		}
		if !active {
			if err := s.cancelTaskForInactiveEnrollment(ctx, task); err != nil {
				failedCount++
			}
			continue
		}

		expiredAt := plan.TaskOpenWindowEndsAt(task.GetPlannedAt())
		if err := s.taskLifecycle.ExpireMissedOpenWindow(task, expiredAt); err != nil {
			failedCount++
			continue
		}
		if err := s.persistence.save(ctx, task, true); err != nil {
			failedCount++
			continue
		}
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
			logger.L(ctx).Errorw("Failed to publish missed-window task event", "task_id", task.GetID().String(), "event_type", evt.EventType(), "error", err.Error())
		})
		expiredCount++
	}
	return expiredCount, failedCount
}

// missedBacklogCount is intentionally a bounded presence signal: the scheduler
// never runs an unbounded COUNT over the task table. A positive value means at
// least one stale pending task remains after this phase.
func (s *taskSchedulerService) missedBacklogCount(ctx context.Context, orgID int64, actionAt time.Time, scanned []*plan.AssessmentTask) (int, error) {
	if !TaskSchedulerMissedExpirationEnabled(ctx) {
		if len(scanned) > 0 {
			return 1, nil
		}
		return 0, nil
	}
	if scope := taskSchedulerScopeFromContext(ctx); scope != nil && strings.TrimSpace(scope.PlanID) != "" {
		if scanner, ok := s.taskRepo.(plan.AssessmentTaskScopedSchedulerRepository); ok {
			planID, err := plan.ParseAssessmentPlanID(strings.TrimSpace(scope.PlanID))
			if err != nil {
				return 0, err
			}
			ids, err := parseScheduleScopeTesteeIDs(scope.TesteeIDs)
			if err != nil {
				return 0, err
			}
			page, err := scanner.FindScopedMissedPendingTaskPage(ctx, orgID, planID, ids, actionAt.Add(-plan.TaskOpenWindow), time.Time{}, 0, 1)
			if err != nil {
				return 0, err
			}
			if len(page) > 0 {
				return 1, nil
			}
			return 0, nil
		}
	}
	scanner, ok := s.taskRepo.(plan.AssessmentTaskSchedulerRepository)
	if !ok {
		for _, task := range scanned {
			if task != nil && task.IsPending() {
				return 1, nil
			}
		}
		return 0, nil
	}
	page, err := scanner.FindMissedPendingTaskPage(ctx, orgID, actionAt.Add(-plan.TaskOpenWindow), time.Time{}, 0, 1)
	if err != nil {
		return 0, err
	}
	if len(page) > 0 {
		return 1, nil
	}
	return 0, nil
}

func oldestTaskAgeSeconds(now time.Time, tasks []*plan.AssessmentTask, taskTime func(*plan.AssessmentTask) time.Time) int64 {
	var oldest time.Time
	for _, task := range tasks {
		if task == nil {
			continue
		}
		candidate := taskTime(task)
		if candidate.IsZero() {
			continue
		}
		if oldest.IsZero() || candidate.Before(oldest) {
			oldest = candidate
		}
	}
	if oldest.IsZero() || !oldest.Before(now) {
		return 0
	}
	return int64(now.Sub(oldest).Seconds())
}

func (s *taskSchedulerService) enrollmentIsActive(ctx context.Context, cache map[string]*plan.Enrollment, task *plan.AssessmentTask) (bool, error) {
	if s.enrollmentRepo == nil {
		return true, nil
	}
	key := task.GetEnrollmentID().String()
	if enrollment, ok := cache[key]; ok {
		return enrollment != nil && enrollment.IsActive() && enrollment.OrgID() == task.GetOrgID(), nil
	}
	enrollment, err := s.enrollmentRepo.FindByID(ctx, task.GetEnrollmentID())
	if err != nil {
		return false, err
	}
	cache[key] = enrollment
	return enrollment != nil && enrollment.IsActive() && enrollment.OrgID() == task.GetOrgID(), nil
}

func (s *taskSchedulerService) cancelTaskForInactiveEnrollment(ctx context.Context, task *plan.AssessmentTask) error {
	if err := s.taskLifecycle.Cancel(ctx, task); err != nil {
		return err
	}
	if err := s.persistence.save(ctx, task, true); err != nil {
		return err
	}
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event while canceling inactive-enrollment task",
			"action", "schedule_pending_tasks",
			"task_id", task.GetID().String(),
			"enrollment_id", task.GetEnrollmentID().String(),
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})
	return nil
}

func (s *taskSchedulerService) loadPlanForTask(
	ctx context.Context,
	cache map[string]*plan.AssessmentPlan,
	task *plan.AssessmentTask,
) (*plan.AssessmentPlan, error) {
	if s.planRepo == nil {
		return nil, nil
	}
	if task == nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "任务不能为空")
	}
	if cache == nil {
		cache = make(map[string]*plan.AssessmentPlan)
	}
	planID := task.GetPlanID()
	cacheKey := planID.String()
	if p, ok := cache[cacheKey]; ok {
		if p == nil || p.GetOrgID() != task.GetOrgID() {
			return nil, errors.WithCode(errorCode.ErrInternalServerError, "任务计划机构不一致")
		}
		return p, nil
	}

	p, err := s.planRepo.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}
	if p.GetOrgID() != task.GetOrgID() {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "任务计划机构不一致")
	}
	cache[cacheKey] = p
	return p, nil
}

func (s *taskSchedulerService) cancelTaskForInactivePlan(
	ctx context.Context,
	task *plan.AssessmentTask,
	parentPlan *plan.AssessmentPlan,
) error {
	if err := s.taskLifecycle.Cancel(ctx, task); err != nil {
		return err
	}
	if err := s.persistence.save(ctx, task, true); err != nil {
		return err
	}
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
		logger.L(ctx).Errorw("Failed to publish task event while canceling inactive-plan task",
			"action", "schedule_pending_tasks",
			"task_id", task.GetID().String(),
			"plan_id", task.GetPlanID().String(),
			"plan_status", parentPlan.GetStatus().String(),
			"event_type", evt.EventType(),
			"error", err.Error(),
		)
	})
	return nil
}
