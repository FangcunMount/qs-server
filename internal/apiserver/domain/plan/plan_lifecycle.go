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

type resumeTaskState struct {
	maxCompletedSeq map[testee.ID]int
	firstTask       map[testee.ID]*AssessmentTask
	tasksByTestee   map[testee.ID][]*AssessmentTask
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
func (l *PlanLifecycle) Activate(_ context.Context, plan *AssessmentPlan) error {
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

	canceledTasks, err := l.cancelOutstandingTasks(ctx, plan, "pause_plan")
	if err != nil {
		return nil, err
	}

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
) (*ResumeTasksResult, error) {
	planID := plan.GetID().String()
	logger.L(ctx).Infow("Resuming plan in domain service",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"current_status", plan.GetStatus().String(),
		"testee_count", len(testeeStartDates),
	)

	if err := validatePlanResume(plan); err != nil {
		return nil, err
	}

	allTasks, err := l.findResumeTasks(ctx, planID, plan)
	if err != nil {
		return nil, err
	}

	state := buildResumeTaskState(allTasks)
	startDateMap := resolveResumeStartDates(plan, state.firstTask, testeeStartDates)
	result, err := l.prepareResumeTasks(ctx, planID, plan, state, startDateMap)
	if err != nil {
		return nil, err
	}

	// 5. 调用聚合根的包内方法（状态变更）
	if err := plan.resume(); err != nil {
		return result, err
	}

	// 注意：领域层不负责持久化，返回待保存的任务供应用层保存
	return result, nil
}

func validatePlanResume(plan *AssessmentPlan) error {
	if plan.IsFinished() {
		return errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能恢复")
	}
	if plan.IsCanceled() {
		return errors.WithCode(code.ErrInvalidArgument, "已取消的计划不能恢复")
	}
	if !plan.IsPaused() {
		return errors.WithCode(code.ErrInvalidArgument, "计划未处于暂停状态，无法恢复")
	}
	return nil
}

func (l *PlanLifecycle) findResumeTasks(ctx context.Context, planID string, plan *AssessmentPlan) ([]*AssessmentTask, error) {
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
	return allTasks, nil
}

func buildResumeTaskState(allTasks []*AssessmentTask) *resumeTaskState {
	state := &resumeTaskState{
		maxCompletedSeq: make(map[testee.ID]int),
		firstTask:       make(map[testee.ID]*AssessmentTask),
		tasksByTestee:   make(map[testee.ID][]*AssessmentTask),
	}

	for _, task := range allTasks {
		testeeID := task.GetTesteeID()
		state.tasksByTestee[testeeID] = append(state.tasksByTestee[testeeID], task)
		if firstTask, exists := state.firstTask[testeeID]; !exists || task.GetSeq() < firstTask.GetSeq() {
			state.firstTask[testeeID] = task
		}
		if task.IsCompleted() && task.GetSeq() > state.maxCompletedSeq[testeeID] {
			state.maxCompletedSeq[testeeID] = task.GetSeq()
		}
	}

	return state
}

func resolveResumeStartDates(
	plan *AssessmentPlan,
	firstTasks map[testee.ID]*AssessmentTask,
	provided map[testee.ID]time.Time,
) map[testee.ID]time.Time {
	startDates := make(map[testee.ID]time.Time, len(firstTasks))
	for testeeID, firstTask := range firstTasks {
		if startDate, ok := provided[testeeID]; ok && !startDate.IsZero() {
			startDates[testeeID] = startDate
			continue
		}

		startDate := inferStartDateFromTask(plan, firstTask)
		if !startDate.IsZero() {
			startDates[testeeID] = startDate
		}
	}
	return startDates
}

func (l *PlanLifecycle) prepareResumeTasks(
	ctx context.Context,
	planID string,
	plan *AssessmentPlan,
	state *resumeTaskState,
	startDates map[testee.ID]time.Time,
) (*ResumeTasksResult, error) {
	result := &ResumeTasksResult{TasksToSave: make([]*AssessmentTask, 0)}
	for testeeID, startDate := range startDates {
		tasks, err := l.prepareResumeTasksForTestee(ctx, planID, plan, state, testeeID, startDate)
		if err != nil {
			return nil, err
		}
		result.TasksToSave = append(result.TasksToSave, tasks...)
	}

	sortTasksBySeq(result.TasksToSave)
	logger.L(ctx).Infow("Tasks prepared for resumed plan",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"tasks_to_save_count", len(result.TasksToSave),
	)
	return result, nil
}

func (l *PlanLifecycle) prepareResumeTasksForTestee(
	ctx context.Context,
	planID string,
	plan *AssessmentPlan,
	state *resumeTaskState,
	testeeID testee.ID,
	startDate time.Time,
) ([]*AssessmentTask, error) {
	if startDate.IsZero() {
		logger.L(ctx).Warnw("Skipping testee with zero start date",
			"domain_action", "resume_plan",
			"plan_id", planID,
			"testee_id", testeeID.String(),
		)
		return nil, nil
	}

	allGeneratedTasks := l.taskGenerator.GenerateTasks(plan, testeeID, startDate)
	existingBySeq := groupTasksBySeq(state.tasksByTestee[testeeID])
	maxCompletedSeq := state.maxCompletedSeq[testeeID]
	tasksToSave := make([]*AssessmentTask, 0, len(allGeneratedTasks))

	for _, task := range allGeneratedTasks {
		if task.GetSeq() <= maxCompletedSeq {
			continue
		}

		reusable := preferredReusableTask(existingBySeq[task.GetSeq()])
		if reusable == nil {
			tasksToSave = append(tasksToSave, task)
			continue
		}

		if err := l.taskLifecycle.Reschedule(ctx, reusable, task.GetPlannedAt()); err != nil {
			logger.L(ctx).Errorw("Failed to reschedule existing task for resumed plan",
				"domain_action", "resume_plan",
				"plan_id", planID,
				"testee_id", testeeID.String(),
				"task_id", reusable.GetID().String(),
				"seq", reusable.GetSeq(),
				"error", err.Error(),
			)
			return nil, errors.WithCode(code.ErrInternalServerError, "重置任务失败: %v", err)
		}
		tasksToSave = append(tasksToSave, reusable)
	}

	logger.L(ctx).Infow("Generated tasks for testee",
		"domain_action", "resume_plan",
		"plan_id", planID,
		"testee_id", testeeID.String(),
		"max_completed_seq", maxCompletedSeq,
		"tasks_to_save_count", len(tasksToSave),
	)
	return tasksToSave, nil
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
// 业务规则：取消时，联动取消所有未执行的任务（pending 和 opened 状态）
func (l *PlanLifecycle) Cancel(ctx context.Context, plan *AssessmentPlan) ([]*AssessmentTask, error) {
	planID := plan.GetID().String()
	logger.L(ctx).Infow("Canceling plan in domain service",
		"domain_action", "cancel_plan",
		"plan_id", planID,
		"current_status", plan.GetStatus().String(),
	)

	// 1. 前置状态检查
	if plan.IsCanceled() {
		return nil, nil // 幂等操作
	}
	if plan.IsFinished() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已完成的计划不能取消")
	}

	canceledTasks, err := l.cancelOutstandingTasks(ctx, plan, "cancel_plan")
	if err != nil {
		return nil, err
	}

	// 2. 调用聚合根的包内方法（状态变更）
	plan.cancel()
	return canceledTasks, nil
}

// Finish 手动结束计划。
// 业务规则：结束计划时，联动取消所有未执行的任务（pending 和 opened 状态）。
func (l *PlanLifecycle) Finish(ctx context.Context, plan *AssessmentPlan) ([]*AssessmentTask, error) {
	planID := plan.GetID().String()
	logger.L(ctx).Infow("Finishing plan in domain service",
		"domain_action", "finish_plan",
		"plan_id", planID,
		"current_status", plan.GetStatus().String(),
	)

	if plan.IsFinished() {
		return nil, nil
	}
	if plan.IsCanceled() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "已取消的计划不能完成")
	}

	canceledTasks, err := l.cancelOutstandingTasks(ctx, plan, "finish_plan")
	if err != nil {
		return nil, err
	}

	plan.finish()
	return canceledTasks, nil
}

func (l *PlanLifecycle) cancelOutstandingTasks(ctx context.Context, plan *AssessmentPlan, action string) ([]*AssessmentTask, error) {
	planID := plan.GetID().String()

	allTasks, err := l.taskRepo.FindByPlanID(ctx, plan.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to find tasks for plan",
			"domain_action", action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrInternalServerError, "查询任务失败: %v", err)
	}

	logger.L(ctx).Infow("Found tasks for plan",
		"domain_action", action,
		"plan_id", planID,
		"total_tasks", len(allTasks),
	)

	var canceledTasks []*AssessmentTask
	for _, task := range allTasks {
		if !task.IsPending() && !task.IsOpened() {
			continue
		}
		if err := l.taskLifecycle.Cancel(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to cancel task",
				"domain_action", action,
				"plan_id", planID,
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			continue
		}
		canceledTasks = append(canceledTasks, task)
	}

	logger.L(ctx).Infow("Outstanding tasks canceled for plan",
		"domain_action", action,
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
	)

	return canceledTasks, nil
}

func preferredReusableTask(tasks []*AssessmentTask) *AssessmentTask {
	var reusable []*AssessmentTask
	for _, task := range tasks {
		if task == nil || task.IsCompleted() {
			continue
		}
		reusable = append(reusable, task)
	}
	return preferredTask(reusable)
}
