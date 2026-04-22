package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
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

func (s *commandService) FinishPlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.lifecycle.FinishPlan(ctx, orgID, planID)
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
	task, err := loadTaskInOrg(ctx, s.taskRepo, orgID, taskID, "cancel_task")
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
	planAggregate, err := loadPlanInOrg(ctx, s.planRepo, orgID, planID, "cancel_plan")
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
		return 0, wrapDatabaseErr(err, "查询计划任务失败")
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
	planAggregate, err := loadPlanInOrg(ctx, s.planRepo, orgID, planID, "terminate_enrollment")
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
		return 0, invalidArgumentErr("无效的受试者ID: %v", err)
	}

	tasks, err := s.taskRepo.FindByTesteeIDAndPlanID(ctx, testeeDomainID, planAggregate.GetID())
	if err != nil {
		logger.L(ctx).Errorw("Failed to load enrollment tasks for command precheck",
			"action", "terminate_enrollment",
			"plan_id", planID,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return 0, wrapDatabaseErr(err, "查询受试者计划任务失败")
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
