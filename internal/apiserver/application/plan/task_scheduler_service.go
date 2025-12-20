package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
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
	// 1. 解析时间参数
	beforeTime, err := parseTime(before)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的时间格式: %v", err)
	}

	// 2. 查询待推送任务
	tasks, err := s.taskRepo.FindPendingTasks(ctx, beforeTime)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询待推送任务失败")
	}

	// 3. 为每个任务生成入口并开放
	var openedTasks []*plan.AssessmentTask
	for _, task := range tasks {
		// 生成入口
		token, url, expireAt, err := s.entryGenerator.GenerateEntry(ctx, task)
		if err != nil {
			// 记录错误但继续处理其他任务
			continue
		}

		// 开放任务
		if err := s.taskLifecycle.Open(ctx, task, token, url, expireAt); err != nil {
			// 记录错误但继续处理其他任务
			continue
		}

		// 持久化任务
		if err := s.taskRepo.Save(ctx, task); err != nil {
			// 记录错误但继续处理其他任务
			continue
		}

		// 发布领域事件
		events := task.Events()
		for _, evt := range events {
			if err := s.eventPublisher.Publish(ctx, evt); err != nil {
				// 记录错误但继续执行
			}
		}
		task.ClearEvents()

		openedTasks = append(openedTasks, task)
	}

	return toTaskResults(openedTasks), nil
}
