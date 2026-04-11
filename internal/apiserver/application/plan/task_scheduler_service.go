package plan

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// taskSchedulerService 任务调度服务实现
// 行为者：任务调度服务
type taskSchedulerService struct {
	taskRepo       plan.AssessmentTaskRepository
	planRepo       plan.AssessmentPlanRepository
	taskLifecycle  *plan.TaskLifecycle
	planLifecycle  *plan.PlanLifecycle
	entryGenerator EntryGenerator // 入口生成器（由基础设施层实现）
	eventPublisher event.EventPublisher
}

// EntryGenerator 入口生成器接口
// 由基础设施层实现，负责生成测评入口（token、URL）
type EntryGenerator interface {
	GenerateEntry(ctx context.Context, task *plan.AssessmentTask) (token string, url string, expireAt time.Time, err error)
}

// NewTaskSchedulerService 创建任务调度服务
func NewTaskSchedulerService(
	taskRepo plan.AssessmentTaskRepository,
	planRepo plan.AssessmentPlanRepository,
	entryGenerator EntryGenerator,
	eventPublisher event.EventPublisher,
) TaskSchedulerService {
	taskGenerator := plan.NewTaskGenerator()
	taskLifecycle := plan.NewTaskLifecycle()
	return &taskSchedulerService{
		taskRepo:       taskRepo,
		planRepo:       planRepo,
		taskLifecycle:  taskLifecycle,
		planLifecycle:  plan.NewPlanLifecycle(taskRepo, taskGenerator, taskLifecycle),
		entryGenerator: entryGenerator,
		eventPublisher: eventPublisher,
	}
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

	// 2. 查询待推送任务
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

	// 3. 为每个任务生成入口并开放
	var openedTasks []*plan.AssessmentTask
	failedCount := 0
	inactivePlanCanceledCount := 0
	planCache := make(map[string]*plan.AssessmentPlan)
	for _, task := range tasks {
		parentPlan, err := s.loadPlanForTask(ctx, planCache, task.GetPlanID())
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

		// 生成入口
		token, url, expireAt, err := s.entryGenerator.GenerateEntry(ctx, task)
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
		if err := s.taskLifecycle.Open(ctx, task, token, url, expireAt); err != nil {
			logger.L(ctx).Errorw("Failed to open task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		// 持久化任务
		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to save opened task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		// 发布领域事件
		events := task.Events()
		for _, evt := range events {
			if err := s.eventPublisher.Publish(ctx, evt); err != nil {
				logger.L(ctx).Errorw("Failed to publish task event",
					"action", "schedule_pending_tasks",
					"task_id", task.GetID().String(),
					"event_type", evt.EventType(),
					"error", err.Error(),
				)
			}
		}
		task.ClearEvents()

		openedTasks = append(openedTasks, task)
	}

	expiredCount := 0
	expireFailedCount := 0
	expiredCount, expireFailedCount = s.expireOverdueTasks(ctx, orgID, planCache)
	CollectTaskScheduleStats(ctx, TaskScheduleStats{
		PendingCount:      len(tasks),
		OpenedCount:       len(openedTasks),
		FailedCount:       failedCount,
		ExpiredCount:      expiredCount,
		ExpireFailedCount: expireFailedCount,
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
	)

	return toTaskResults(openedTasks), nil
}

func (s *taskSchedulerService) findPendingTasks(ctx context.Context, orgID int64, before time.Time) ([]*plan.AssessmentTask, error) {
	scope := taskSchedulerScopeFromContext(ctx)
	if scope == nil || (strings.TrimSpace(scope.PlanID) == "" && len(scope.TesteeIDs) == 0) {
		return s.taskRepo.FindPendingTasks(ctx, orgID, before)
	}
	planID := strings.TrimSpace(scope.PlanID)
	if planID == "" {
		return s.taskRepo.FindPendingTasks(ctx, orgID, before)
	}

	parsedPlanID, err := plan.ParseAssessmentPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID")
	}

	var tasks []*plan.AssessmentTask
	if len(scope.TesteeIDs) > 0 {
		testeeIDs, err := parseScheduleScopeTesteeIDs(scope.TesteeIDs)
		if err != nil {
			return nil, err
		}
		tasks, err = s.taskRepo.FindByPlanIDAndTesteeIDs(ctx, parsedPlanID, testeeIDs)
		if err != nil {
			return nil, err
		}
	} else {
		tasks, err = s.taskRepo.FindByPlanID(ctx, parsedPlanID)
		if err != nil {
			return nil, err
		}
	}

	filtered := make([]*plan.AssessmentTask, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.GetOrgID() != orgID || !task.IsPending() {
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
	return filtered, nil
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

func (s *taskSchedulerService) expireOverdueTasks(ctx context.Context, orgID int64, planCache map[string]*plan.AssessmentPlan) (int, int) {
	tasks, err := s.taskRepo.FindExpiredTasks(ctx)
	if err != nil {
		logger.L(ctx).Errorw("Failed to find expired tasks",
			"action", "schedule_pending_tasks",
			"error", err.Error(),
		)
		return 0, 1
	}

	expiredCount := 0
	failedCount := 0
	affectedPlans := make(map[string]plan.AssessmentPlanID)
	for _, task := range tasks {
		if task.GetOrgID() != orgID {
			continue
		}
		parentPlan, err := s.loadPlanForTask(ctx, planCache, task.GetPlanID())
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
		if err := s.taskLifecycle.Expire(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to expire task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to save expired task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			failedCount++
			continue
		}

		for _, evt := range task.Events() {
			if err := s.eventPublisher.Publish(ctx, evt); err != nil {
				logger.L(ctx).Errorw("Failed to publish expired task event",
					"action", "schedule_pending_tasks",
					"task_id", task.GetID().String(),
					"event_type", evt.EventType(),
					"error", err.Error(),
				)
			}
		}
		task.ClearEvents()

		affectedPlans[task.GetPlanID().String()] = task.GetPlanID()
		expiredCount++
	}

	for _, planID := range affectedPlans {
		if err := s.finishPlanIfDone(ctx, planID); err != nil {
			logger.L(ctx).Warnw("Failed to finalize plan after expiring tasks",
				"action", "schedule_pending_tasks",
				"plan_id", planID.String(),
				"error", err.Error(),
			)
		}
	}

	return expiredCount, failedCount
}

func (s *taskSchedulerService) loadPlanForTask(
	ctx context.Context,
	cache map[string]*plan.AssessmentPlan,
	planID plan.AssessmentPlanID,
) (*plan.AssessmentPlan, error) {
	if s.planRepo == nil {
		return nil, nil
	}
	if cache == nil {
		cache = make(map[string]*plan.AssessmentPlan)
	}
	cacheKey := planID.String()
	if p, ok := cache[cacheKey]; ok {
		return p, nil
	}

	p, err := s.planRepo.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
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
	if err := s.taskRepo.Save(ctx, task); err != nil {
		return err
	}
	for _, evt := range task.Events() {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish task event while canceling inactive-plan task",
				"action", "schedule_pending_tasks",
				"task_id", task.GetID().String(),
				"plan_id", task.GetPlanID().String(),
				"plan_status", parentPlan.GetStatus().String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	task.ClearEvents()
	return nil
}

func (s *taskSchedulerService) finishPlanIfDone(ctx context.Context, planID plan.AssessmentPlanID) error {
	return finalizePlanIfDone(
		ctx,
		"finish_plan_after_task_scheduling",
		s.planRepo,
		s.planLifecycle,
		s.eventPublisher,
		planID,
	)
}
