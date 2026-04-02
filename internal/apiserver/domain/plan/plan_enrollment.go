package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PlanEnrollment 受试者加入计划领域服务
// 负责控制受试者加入测评计划：加入计划、终止计划
// 注意：领域层不负责持久化，持久化由应用层负责
type PlanEnrollment struct {
	planRepo      AssessmentPlanRepository
	taskRepo      AssessmentTaskRepository
	taskGenerator *TaskGenerator
	validator     *PlanValidator
}

// NewPlanEnrollment 创建加入计划服务
func NewPlanEnrollment(
	planRepo AssessmentPlanRepository,
	taskRepo AssessmentTaskRepository,
	taskGenerator *TaskGenerator,
	validator *PlanValidator,
) *PlanEnrollment {
	return &PlanEnrollment{
		planRepo:      planRepo,
		taskRepo:      taskRepo,
		taskGenerator: taskGenerator,
		validator:     validator,
	}
}

// EnrollTestee 将受试者加入计划，并生成任务
//
// 参数：
//   - planID: 测评计划ID
//   - testeeID: 受试者ID
//   - startDate: 基准日期，所有相对时间都基于此日期计算
//
// 返回：
//   - tasks: 生成的任务列表
//   - error: 错误信息
func (e *PlanEnrollment) EnrollTestee(
	ctx context.Context,
	planID AssessmentPlanID,
	testeeID testee.ID,
	startDate time.Time,
) (*EnrollmentTasksResult, error) {
	logger.L(ctx).Infow("Enrolling testee in domain service",
		"domain_action", "enroll_testee",
		"plan_id", planID.String(),
		"testee_id", testeeID.String(),
		"start_date", startDate,
	)

	// 1. 查询计划
	plan, err := e.planRepo.FindByID(ctx, planID)
	if err != nil {
		logger.L(ctx).Errorw("Plan not found",
			"domain_action", "enroll_testee",
			"plan_id", planID.String(),
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrPageNotFound, "计划不存在")
	}

	// 2. 验证参数
	if testeeID.IsZero() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "受试者ID不能为空")
	}
	if startDate.IsZero() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "开始日期不能为空")
	}

	// 3. 验证计划是否可以加入
	if errs := e.validator.ValidateForEnrollment(plan, testeeID, startDate); len(errs) > 0 {
		logger.L(ctx).Errorw("Validation failed for enrollment",
			"domain_action", "enroll_testee",
			"plan_id", planID.String(),
			"testee_id", testeeID.String(),
			"errors", errs,
		)
		return nil, ToError(errs)
	}

	// 4. 生成期望任务
	expectedTasks := e.taskGenerator.GenerateTasks(plan, testeeID, startDate)
	if len(expectedTasks) == 0 {
		logger.L(ctx).Errorw("No tasks generated",
			"domain_action", "enroll_testee",
			"plan_id", planID.String(),
			"testee_id", testeeID.String(),
		)
		return nil, errors.WithCode(code.ErrInvalidArgument, "未能生成任何任务")
	}

	logger.L(ctx).Infow("Expected tasks generated for enrollment",
		"domain_action", "enroll_testee",
		"plan_id", planID.String(),
		"testee_id", testeeID.String(),
		"tasks_count", len(expectedTasks),
	)

	// 5. 检查已有任务，实现 enroll 幂等
	existingTasks, err := e.taskRepo.FindByTesteeIDAndPlanID(ctx, testeeID, planID)
	if err != nil {
		logger.L(ctx).Errorw("Failed to find existing enrollment tasks",
			"domain_action", "enroll_testee",
			"plan_id", planID.String(),
			"testee_id", testeeID.String(),
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrInternalServerError, "查询任务失败: %v", err)
	}

	result := &EnrollmentTasksResult{
		Tasks:       make([]*AssessmentTask, 0, len(expectedTasks)),
		TasksToSave: make([]*AssessmentTask, 0, len(expectedTasks)),
	}
	if len(existingTasks) == 0 {
		result.Tasks = expectedTasks
		result.TasksToSave = expectedTasks
		return result, nil
	}

	candidatesBySeq := groupTasksBySeq(existingTasks)
	for _, expectedTask := range expectedTasks {
		candidates := candidatesBySeq[expectedTask.GetSeq()]

		var matchingCandidates []*AssessmentTask
		for _, candidate := range candidates {
			if taskMatchesExpectedSchedule(candidate, expectedTask) {
				matchingCandidates = append(matchingCandidates, candidate)
			}
		}

		if len(matchingCandidates) > 0 {
			result.Tasks = append(result.Tasks, preferredTask(matchingCandidates))
			delete(candidatesBySeq, expectedTask.GetSeq())
			continue
		}

		if len(candidates) > 0 {
			return nil, errors.WithCode(code.ErrInvalidArgument, "受试者已加入此计划，且开始日期与现有任务不一致")
		}

		result.Tasks = append(result.Tasks, expectedTask)
		result.TasksToSave = append(result.TasksToSave, expectedTask)
	}

	if len(candidatesBySeq) > 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "受试者已加入此计划，且现有任务与计划定义不一致")
	}

	sortTasksBySeq(result.Tasks)
	sortTasksBySeq(result.TasksToSave)
	result.Idempotent = len(result.TasksToSave) == 0

	// 注意：领域层不负责持久化，返回任务调和结果供应用层保存。
	// enrollment 生命周期事件由应用层在持久化成功后发布。

	return result, nil
}

// TerminateEnrollment 终止受试者的计划参与
// 取消该受试者在该计划下的所有待处理任务
//
// 参数：
//   - planID: 测评计划ID
//   - testeeID: 受试者ID
//
// 返回：
//   - canceledTasks: 被取消的任务列表
//   - error: 错误信息
func (e *PlanEnrollment) TerminateEnrollment(
	ctx context.Context,
	planID AssessmentPlanID,
	testeeID testee.ID,
) ([]*AssessmentTask, error) {
	logger.L(ctx).Infow("Terminating enrollment in domain service",
		"domain_action", "terminate_enrollment",
		"plan_id", planID.String(),
		"testee_id", testeeID.String(),
	)

	// 1. 查询该受试者的所有任务
	allTasks, err := e.taskRepo.FindByPlanID(ctx, planID)
	if err != nil {
		logger.L(ctx).Errorw("Failed to find tasks",
			"domain_action", "terminate_enrollment",
			"plan_id", planID.String(),
			"error", err.Error(),
		)
		return nil, errors.WithCode(code.ErrInternalServerError, "查询任务失败: %v", err)
	}

	// 3. 过滤出该受试者的任务
	var testeeTasks []*AssessmentTask
	for _, task := range allTasks {
		if task.GetTesteeID() == testeeID {
			testeeTasks = append(testeeTasks, task)
		}
	}

	logger.L(ctx).Infow("Found tasks for testee",
		"domain_action", "terminate_enrollment",
		"plan_id", planID.String(),
		"testee_id", testeeID.String(),
		"testee_tasks_count", len(testeeTasks),
	)

	// 4. 取消所有非终态的任务
	var canceledTasks []*AssessmentTask
	taskLifecycle := NewTaskLifecycle()
	for _, task := range testeeTasks {
		if !task.IsTerminal() {
			if err := taskLifecycle.Cancel(ctx, task); err != nil {
				logger.L(ctx).Errorw("Failed to cancel task",
					"domain_action", "terminate_enrollment",
					"task_id", task.GetID().String(),
					"error", err.Error(),
				)
				continue
			}
			canceledTasks = append(canceledTasks, task)
		}
	}

	logger.L(ctx).Infow("Tasks canceled for terminated enrollment",
		"domain_action", "terminate_enrollment",
		"plan_id", planID.String(),
		"testee_id", testeeID.String(),
		"canceled_tasks_count", len(canceledTasks),
	)

	// 注意：领域层不负责持久化，返回被取消的任务供应用层保存。
	// enrollment 生命周期事件由应用层在持久化成功后发布。

	return canceledTasks, nil
}
