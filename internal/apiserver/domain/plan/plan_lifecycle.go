package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PlanLifecycle 计划生命周期管理领域服务
// 负责控制测评计划的生命周期：开启、取消、暂停、恢复
// 业务规则：
// - 暂停计划时，取消所有未执行的任务（pending 和 opened 状态）
// - 恢复计划时，为每个受试者重新生成未完成的任务
type PlanLifecycle struct {
	taskRepo      AssessmentTaskRepository
	taskGenerator *TaskGenerator
	taskLifecycle *TaskLifecycle
}

// NewPlanLifecycle 创建计划生命周期管理器
func NewPlanLifecycle(
	taskRepo AssessmentTaskRepository,
	taskGenerator *TaskGenerator,
	taskLifecycle *TaskLifecycle,
) *PlanLifecycle {
	return &PlanLifecycle{
		taskRepo:      taskRepo,
		taskGenerator: taskGenerator,
		taskLifecycle: taskLifecycle,
	}
}

// Activate 开启计划（将计划设置为活跃状态）
// 适用于：从暂停状态恢复，或新创建的计划
func (l *PlanLifecycle) Activate(ctx context.Context, plan *AssessmentPlan) error {
	// 1. 前置状态检查
	if plan.IsFinished() {
		return errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能开启")
	}
	if plan.IsCanceled() {
		return errors.WithCode(code.ErrInvalidArgument, "已取消的计划不能开启")
	}
	if plan.IsActive() {
		return nil // 已经是活跃状态，幂等操作
	}

	// 2. 如果是暂停状态，则恢复
	if plan.IsPaused() {
		return plan.resume()
	}

	// 3. 其他情况，设置为活跃状态
	plan.status = PlanStatusActive
	return nil
}

// Pause 暂停计划（将活跃状态的计划变更为暂停状态）
// 业务规则：暂停时，取消所有未执行的任务（pending 和 opened 状态）
func (l *PlanLifecycle) Pause(ctx context.Context, plan *AssessmentPlan) ([]*AssessmentTask, error) {
	planID := plan.GetID().String()
	logger.L(ctx).Infow("Pausing plan in domain service",
		"domain_action", "pause_plan",
		"plan_id", planID,
		"current_status", plan.GetStatus().String(),
	)

	// 1. 前置状态检查
	if plan.IsFinished() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能暂停")
	}
	if plan.IsCanceled() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已取消的计划不能暂停")
	}
	if !plan.IsActive() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "计划未处于活跃状态，无法暂停")
	}

	// 2. 查询该计划的所有任务
	allTasks, err := l.taskRepo.FindByPlanID(ctx, plan.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to find tasks for plan",
			"domain_action", "pause_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrInternalServerError, "查询任务失败: %v", err)
	}

	logger.L(ctx).Infow("Found tasks for plan",
		"domain_action", "pause_plan",
		"plan_id", planID,
		"total_tasks", len(allTasks),
	)

	// 3. 取消所有未执行的任务（pending 和 opened 状态）
	var canceledTasks []*AssessmentTask
	for _, task := range allTasks {
		if task.IsPending() || task.IsOpened() {
			if err := l.taskLifecycle.Cancel(ctx, task); err != nil {
				logger.L(ctx).Errorw("Failed to cancel task",
					"domain_action", "pause_plan",
					"plan_id", planID,
					"task_id", task.GetID().String(),
					"error", err.Error(),
				)
				continue
			}
			canceledTasks = append(canceledTasks, task)
		}
	}

	logger.L(ctx).Infow("Tasks canceled for paused plan",
		"domain_action", "pause_plan",
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
	)

	// 4. 调用聚合根的包内方法（状态变更）
	if err := plan.pause(); err != nil {
		return canceledTasks, err
	}

	// 注意：领域层不负责持久化，返回被取消的任务供应用层保存
	return canceledTasks, nil
}

// Resume 恢复计划（将暂停状态的计划变更为活跃状态）
// 业务规则：恢复时，为每个受试者重新生成未完成的任务
// 参数：
//   - testeeStartDates: 受试者ID到开始日期的映射，用于重新生成任务
//     如果某个受试者没有提供 startDate，则从已存在的任务中推断
func (l *PlanLifecycle) Resume(
	ctx context.Context,
	plan *AssessmentPlan,
	testeeStartDates map[testee.ID]time.Time,
) ([]*AssessmentTask, error) {
	planID := plan.GetID().String()
	logger.L(ctx).Infow("Resuming plan in domain service",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"current_status", plan.GetStatus().String(),
		"testee_count", len(testeeStartDates),
	)

	// 1. 前置状态检查
	if plan.IsFinished() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能恢复")
	}
	if plan.IsCanceled() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已取消的计划不能恢复")
	}
	if !plan.IsPaused() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "计划未处于暂停状态，无法恢复")
	}

	// 2. 查询该计划的所有任务
	allTasks, err := l.taskRepo.FindByPlanID(ctx, plan.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to find tasks for plan",
			"domain_action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrInternalServerError, "查询任务失败: %v", err)
	}

	logger.L(ctx).Infow("Found tasks for plan",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"total_tasks", len(allTasks),
	)

	// 3. 按受试者分组，找出每个受试者已完成的最大序号和开始日期
	testeeMaxSeq := make(map[testee.ID]int)
	testeeStartDateMap := make(map[testee.ID]time.Time)
	testeeFirstTask := make(map[testee.ID]*AssessmentTask)

	// 找出每个受试者的第一个任务（用于推断 startDate）
	for _, task := range allTasks {
		testeeID := task.GetTesteeID()
		if firstTask, exists := testeeFirstTask[testeeID]; !exists || task.GetSeq() < firstTask.GetSeq() {
			testeeFirstTask[testeeID] = task
		}

		// 找出已完成任务的最大序号
		if task.IsCompleted() {
			if seq := task.GetSeq(); seq > testeeMaxSeq[testeeID] {
				testeeMaxSeq[testeeID] = seq
			}
		}
	}

	// 为每个受试者确定 startDate
	for testeeID, firstTask := range testeeFirstTask {
		// 如果提供了 startDate，使用提供的
		if startDate, ok := testeeStartDates[testeeID]; ok && !startDate.IsZero() {
			testeeStartDateMap[testeeID] = startDate
		} else {
			// 从第一个任务推断 startDate
			startDate := inferStartDateFromTask(plan, firstTask)
			if !startDate.IsZero() {
				testeeStartDateMap[testeeID] = startDate
			}
		}
	}

	// 4. 为每个受试者重新生成未完成的任务
	var newTasks []*AssessmentTask
	for testeeID, startDate := range testeeStartDateMap {
		if startDate.IsZero() {
			logger.L(ctx).Warnw("Skipping testee with zero start date",
				"domain_action", "resume_plan",
				"plan_id", planID,
				"testee_id", testeeID.String(),
			)
			continue
		}

		// 生成所有任务
		allGeneratedTasks := l.taskGenerator.GenerateTasks(plan, testeeID, startDate)

		// 找出需要重新生成的任务（序号大于已完成的最大序号）
		maxCompletedSeq := testeeMaxSeq[testeeID]
		for _, task := range allGeneratedTasks {
			if task.GetSeq() > maxCompletedSeq {
				newTasks = append(newTasks, task)
			}
		}

		logger.L(ctx).Infow("Generated tasks for testee",
			"domain_action", "resume_plan",
			"plan_id", planID,
			"testee_id", testeeID.String(),
			"max_completed_seq", maxCompletedSeq,
			"new_tasks_count", len(allGeneratedTasks)-maxCompletedSeq,
		)
	}

	logger.L(ctx).Infow("New tasks generated for resumed plan",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"new_tasks_count", len(newTasks),
	)

	// 5. 调用聚合根的包内方法（状态变更）
	if err := plan.resume(); err != nil {
		return newTasks, err
	}

	// 注意：领域层不负责持久化，返回新生成的任务供应用层保存
	return newTasks, nil
}

// inferStartDateFromTask 从任务推断开始日期
// 根据计划的周期类型和任务的序号、计划时间点，反推 startDate
func inferStartDateFromTask(plan *AssessmentPlan, task *AssessmentTask) time.Time {
	plannedAt := task.GetPlannedAt()
	seq := task.GetSeq()

	switch plan.GetScheduleType() {
	case PlanScheduleByWeek:
		// plannedAt = startDate + (seq-1) * interval * 7 天
		// startDate = plannedAt - (seq-1) * interval * 7 天
		daysOffset := (seq - 1) * plan.GetInterval() * 7
		return plannedAt.AddDate(0, 0, -daysOffset)

	case PlanScheduleByDay:
		// plannedAt = startDate + (seq-1) * interval 天
		// startDate = plannedAt - (seq-1) * interval 天
		daysOffset := (seq - 1) * plan.GetInterval()
		return plannedAt.AddDate(0, 0, -daysOffset)

	case PlanScheduleCustom:
		// plannedAt = startDate + relativeWeeks[seq-1] * 7 天
		// startDate = plannedAt - relativeWeeks[seq-1] * 7 天
		relativeWeeks := plan.GetRelativeWeeks()
		if seq > 0 && seq <= len(relativeWeeks) {
			weeksOffset := relativeWeeks[seq-1]
			return plannedAt.AddDate(0, 0, -weeksOffset*7)
		}
		// 如果序号超出范围，返回第一个任务的 plannedAt
		return plannedAt

	case PlanScheduleFixedDate:
		// 固定日期类型，第一个任务的 plannedAt 就是 startDate
		return plannedAt

	default:
		return plannedAt
	}
}

// Cancel 取消计划（将计划变更为已取消状态）
func (l *PlanLifecycle) Cancel(ctx context.Context, plan *AssessmentPlan) error {
	// 1. 前置状态检查
	if plan.IsCanceled() {
		return nil // 幂等操作
	}
	if plan.IsFinished() {
		return errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能取消")
	}

	// 2. 调用聚合根的包内方法（状态变更）
	plan.cancel()
	return nil
}
