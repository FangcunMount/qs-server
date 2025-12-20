package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// taskSchedulerService 任务调度服务实现
// 行为者：任务调度服务
type taskSchedulerService struct {
	taskRepo       plan.AssessmentTaskRepository
	taskLifecycle  *plan.TaskLifecycle
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
	entryGenerator EntryGenerator,
	eventPublisher event.EventPublisher,
) TaskSchedulerService {
	return &taskSchedulerService{
		taskRepo:       taskRepo,
		taskLifecycle:  plan.NewTaskLifecycle(),
		entryGenerator: entryGenerator,
		eventPublisher: eventPublisher,
	}
}

// SchedulePendingTasks 调度待推送的任务
func (s *taskSchedulerService) SchedulePendingTasks(ctx context.Context, before string) ([]*TaskResult, error) {
	logger.L(ctx).Infow("Scheduling pending tasks",
		"action", "schedule_pending_tasks",
		"before", before,
	)

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
	tasks, err := s.taskRepo.FindPendingTasks(ctx, beforeTime)
	if err != nil {
		logger.L(ctx).Errorw("Failed to find pending tasks",
			"action", "schedule_pending_tasks",
			"before", before,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询待推送任务失败")
	}

	logger.L(ctx).Infow("Found pending tasks",
		"action", "schedule_pending_tasks",
		"before", before,
		"pending_tasks_count", len(tasks),
	)

	// 3. 为每个任务生成入口并开放
	var openedTasks []*plan.AssessmentTask
	failedCount := 0
	for _, task := range tasks {
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

	logger.L(ctx).Infow("Tasks scheduled",
		"action", "schedule_pending_tasks",
		"before", before,
		"total_pending", len(tasks),
		"opened_count", len(openedTasks),
		"failed_count", failedCount,
	)

	return toTaskResults(openedTasks), nil
}
