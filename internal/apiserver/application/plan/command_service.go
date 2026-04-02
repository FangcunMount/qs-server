package plan

import (
	"context"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type commandService struct {
	lifecycle      PlanLifecycleService
	enrollment     PlanEnrollmentService
	taskScheduler  TaskSchedulerService
	taskManagement TaskManagementService
	planRepo       domainplan.AssessmentPlanRepository
	taskRepo       domainplan.AssessmentTaskRepository
}

// NewCommandService 收敛 plan 写侧命令入口。
func NewCommandService(
	lifecycle PlanLifecycleService,
	enrollment PlanEnrollmentService,
	taskScheduler TaskSchedulerService,
	taskManagement TaskManagementService,
	planRepo domainplan.AssessmentPlanRepository,
	taskRepo domainplan.AssessmentTaskRepository,
) PlanCommandService {
	return &commandService{
		lifecycle:      lifecycle,
		enrollment:     enrollment,
		taskScheduler:  taskScheduler,
		taskManagement: taskManagement,
		planRepo:       planRepo,
		taskRepo:       taskRepo,
	}
}

func (s *commandService) CreatePlan(ctx context.Context, dto CreatePlanDTO) (*PlanResult, error) {
	return s.lifecycle.CreatePlan(ctx, dto)
}

func (s *commandService) PausePlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.lifecycle.PausePlan(ctx, orgID, planID)
}

func (s *commandService) ResumePlan(ctx context.Context, orgID int64, planID string, testeeStartDates map[string]string) (*PlanResult, error) {
	return s.lifecycle.ResumePlan(ctx, orgID, planID, testeeStartDates)
}

func (s *commandService) CancelPlan(ctx context.Context, orgID int64, planID string) (*PlanMutationResult, error) {
	affectedTaskCount, err := s.countCancelableTasksByPlan(ctx, orgID, planID)
	if err != nil {
		return nil, err
	}
	if err := s.lifecycle.CancelPlan(ctx, orgID, planID); err != nil {
		return nil, err
	}
	return &PlanMutationResult{
		PlanID:            planID,
		AffectedTaskCount: affectedTaskCount,
	}, nil
}

func (s *commandService) EnrollTestee(ctx context.Context, dto EnrollTesteeDTO) (*EnrollmentResult, error) {
	return s.enrollment.EnrollTestee(ctx, dto)
}

func (s *commandService) TerminateEnrollment(ctx context.Context, orgID int64, planID string, testeeID string) (*EnrollmentTerminationResult, error) {
	affectedTaskCount, err := s.countCancelableTasksByEnrollment(ctx, orgID, planID, testeeID)
	if err != nil {
		return nil, err
	}
	if err := s.enrollment.TerminateEnrollment(ctx, orgID, planID, testeeID); err != nil {
		return nil, err
	}
	return &EnrollmentTerminationResult{
		PlanID:            planID,
		TesteeID:          testeeID,
		AffectedTaskCount: affectedTaskCount,
	}, nil
}

func (s *commandService) SchedulePendingTasks(ctx context.Context, orgID int64, before string) (*TaskScheduleResult, error) {
	stats := &TaskScheduleStats{}
	scheduleCtx := WithTaskScheduleStatsCollector(ctx, stats)
	tasks, err := s.taskScheduler.SchedulePendingTasks(scheduleCtx, orgID, before)
	if err != nil {
		return nil, err
	}
	return &TaskScheduleResult{
		Tasks: tasks,
		Stats: *stats,
	}, nil
}

func (s *commandService) OpenTask(ctx context.Context, orgID int64, taskID string, dto OpenTaskDTO) (*TaskResult, error) {
	return s.taskManagement.OpenTask(ctx, orgID, taskID, dto)
}

func (s *commandService) CompleteTask(ctx context.Context, orgID int64, taskID string, assessmentID string) (*TaskResult, error) {
	return s.taskManagement.CompleteTask(ctx, orgID, taskID, assessmentID)
}

func (s *commandService) ExpireTask(ctx context.Context, orgID int64, taskID string) (*TaskResult, error) {
	return s.taskManagement.ExpireTask(ctx, orgID, taskID)
}

func (s *commandService) CancelTask(ctx context.Context, orgID int64, taskID string) (*TaskMutationResult, error) {
	task, err := s.loadTaskInOrg(ctx, orgID, taskID, "cancel_task")
	if err != nil {
		return nil, err
	}
	if err := s.taskManagement.CancelTask(ctx, orgID, taskID); err != nil {
		return nil, err
	}
	return &TaskMutationResult{
		TaskID:            taskID,
		PlanID:            task.GetPlanID().String(),
		AffectedTaskCount: 1,
	}, nil
}

func (s *commandService) countCancelableTasksByPlan(ctx context.Context, orgID int64, planID string) (int, error) {
	planAggregate, err := s.loadPlanInOrg(ctx, orgID, planID, "cancel_plan")
	if err != nil {
		return 0, err
	}

	tasks, err := s.taskRepo.FindByPlanID(ctx, planAggregate.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to load plan tasks for command precheck",
			"action", "cancel_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return 0, pkgerrors.WrapC(err, errorCode.ErrDatabase, "查询计划任务失败")
	}

	count := 0
	for _, task := range tasks {
		if task == nil || task.GetOrgID() != orgID {
			continue
		}
		if task.IsPending() || task.IsOpened() {
			count++
		}
	}
	return count, nil
}

func (s *commandService) countCancelableTasksByEnrollment(ctx context.Context, orgID int64, planID string, testeeID string) (int, error) {
	planAggregate, err := s.loadPlanInOrg(ctx, orgID, planID, "terminate_enrollment")
	if err != nil {
		return 0, err
	}

	testeeDomainID, err := toTesteeID(testeeID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid testee ID",
			"action", "terminate_enrollment",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return 0, pkgerrors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	tasks, err := s.taskRepo.FindByTesteeIDAndPlanID(ctx, testeeDomainID, planAggregate.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to load enrollment tasks for command precheck",
			"action", "terminate_enrollment",
			"plan_id", planID,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return 0, pkgerrors.WrapC(err, errorCode.ErrDatabase, "查询受试者计划任务失败")
	}

	count := 0
	for _, task := range tasks {
		if task == nil || task.GetOrgID() != orgID {
			continue
		}
		if task.IsPending() || task.IsOpened() {
			count++
		}
	}
	return count, nil
}

func (s *commandService) loadPlanInOrg(ctx context.Context, orgID int64, planID string, action string) (*domainplan.AssessmentPlan, error) {
	id, err := toPlanID(planID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	planAggregate, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Plan not found",
			"action", action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	if planAggregate.GetOrgID() != orgID {
		logger.L(ctx).Warnw("Plan access denied due to org scope mismatch",
			"action", action,
			"plan_id", planID,
			"request_org_id", orgID,
			"resource_org_id", planAggregate.GetOrgID(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrPermissionDenied, "计划不属于当前机构")
	}

	return planAggregate, nil
}

func (s *commandService) loadTaskInOrg(ctx context.Context, orgID int64, taskID string, action string) (*domainplan.AssessmentTask, error) {
	id, err := toTaskID(taskID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid task ID",
			"action", action,
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrInvalidArgument, "无效的任务ID: %v", err)
	}

	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Task not found",
			"action", action,
			"task_id", taskID,
			"error", err.Error(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
	}

	if task.GetOrgID() != orgID {
		logger.L(ctx).Warnw("Task access denied due to org scope mismatch",
			"action", action,
			"task_id", taskID,
			"request_org_id", orgID,
			"resource_org_id", task.GetOrgID(),
		)
		return nil, pkgerrors.WithCode(errorCode.ErrPermissionDenied, "任务不属于当前机构")
	}

	return task, nil
}
