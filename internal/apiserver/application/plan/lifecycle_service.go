package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 计划生命周期服务实现
// 行为者：计划管理员
type lifecycleService struct {
	planRepo           plan.AssessmentPlanRepository
	taskRepo           plan.AssessmentTaskRepository
	lifecycle          *plan.PlanLifecycle
	createWorkflow     *planCreateWorkflow
	transitionWorkflow *planTransitionWorkflow
}

// NewLifecycleService 创建计划生命周期服务
func NewLifecycleService(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	scaleRepo scale.Repository,
	eventPublisher event.EventPublisher,
) PlanLifecycleService {
	return NewLifecycleServiceWithScaleCatalog(planRepo, taskRepo, newRepositoryScaleCatalog(scaleRepo), eventPublisher)
}

// NewLifecycleServiceWithScaleCatalog 创建使用 scale catalog 防腐接口的计划生命周期服务。
func NewLifecycleServiceWithScaleCatalog(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	scaleCatalog ScaleCatalog,
	eventPublisher event.EventPublisher,
) PlanLifecycleService {
	taskGenerator := plan.NewTaskGenerator()
	taskLifecycle := plan.NewTaskLifecycle()
	lifecycle := plan.NewPlanLifecycle(taskRepo, taskGenerator, taskLifecycle)

	return &lifecycleService{
		planRepo:           planRepo,
		taskRepo:           taskRepo,
		lifecycle:          lifecycle,
		createWorkflow:     newPlanCreateWorkflow(planRepo, scaleCatalog, plan.NewPlanValidator()),
		transitionWorkflow: newPlanTransitionWorkflow(planRepo, taskRepo, eventPublisher),
	}
}

// CreatePlan 创建测评计划模板
func (s *lifecycleService) CreatePlan(ctx context.Context, dto CreatePlanDTO) (*PlanResult, error) {
	planAggregate, err := s.createWorkflow.create(ctx, dto)
	if err != nil {
		return nil, err
	}
	return toPlanResult(planAggregate), nil
}

// PausePlan 暂停计划
func (s *lifecycleService) PausePlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.transitionWorkflow.transitionPlanWithTaskCancellation(
		ctx,
		orgID,
		planID,
		planTransitionSpec{
			action:          "pause_plan",
			startLog:        "Pausing assessment plan",
			transitionLog:   "Plan paused, canceling tasks",
			transitionError: "Failed to pause plan",
			planSaveError:   "Failed to save paused plan",
			taskSaveError:   "Failed to save canceled task",
			successLog:      "Plan paused successfully",
		},
		s.lifecycle.Pause,
	)
}

// ResumePlan 恢复计划
func (s *lifecycleService) ResumePlan(ctx context.Context, orgID int64, planID string, testeeStartDates map[string]string) (*PlanResult, error) {
	logger.L(ctx).Infow("Resuming assessment plan",
		"action", "resume_plan",
		"org_id", orgID,
		"plan_id", planID,
		"testee_count", len(testeeStartDates),
	)

	// 1. 查询并校验计划
	p, err := loadPlanInOrg(ctx, s.planRepo, orgID, planID, "resume_plan")
	if err != nil {
		return nil, err
	}

	// 转换 testeeStartDates
	testeeStartDateMap := make(map[testee.ID]time.Time)
	for testeeIDStr, dateStr := range testeeStartDates {
		testeeID, err := toTesteeID(testeeIDStr)
		if err != nil {
			continue // 跳过无效的受试者ID
		}
		date, err := parseDate(dateStr)
		if err != nil {
			continue // 跳过无效的日期
		}
		testeeStartDateMap[testeeID] = date
	}

	// 2. 调用领域服务恢复计划
	resumeResult, err := s.lifecycle.Resume(ctx, p, testeeStartDateMap)
	if err != nil {
		logger.L(ctx).Errorw("Failed to resume plan",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow("Plan resumed, preparing outstanding tasks",
		"action", "resume_plan",
		"plan_id", planID,
		"tasks_to_save_count", len(resumeResult.TasksToSave),
	)

	// 3. 持久化计划
	if err := s.planRepo.Save(ctx, p); err != nil {
		logger.L(ctx).Errorw("Failed to save resumed plan",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 4. 持久化恢复后的任务（包含新生成任务和复用重置任务）
	savedTaskCount := 0
	for _, task := range resumeResult.TasksToSave {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to save resumed task",
				"action", "resume_plan",
				"plan_id", planID,
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
		savedTaskCount++
	}

	logger.L(ctx).Infow("Plan resumed successfully",
		"action", "resume_plan",
		"plan_id", planID,
		"tasks_to_save_count", len(resumeResult.TasksToSave),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(p), nil
}

// FinishPlan 手动结束计划
func (s *lifecycleService) FinishPlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.transitionWorkflow.transitionPlanWithTaskCancellation(
		ctx,
		orgID,
		planID,
		planTransitionSpec{
			action:          "finish_plan",
			startLog:        "Finishing assessment plan",
			transitionLog:   "Plan finished, canceling outstanding tasks",
			transitionError: "Failed to finish plan",
			planSaveError:   "Failed to save finished plan",
			taskSaveError:   "Failed to save canceled task while finishing plan",
			successLog:      "Plan finished successfully",
		},
		s.lifecycle.Finish,
	)
}

// CancelPlan 取消计划
func (s *lifecycleService) CancelPlan(ctx context.Context, orgID int64, planID string) error {
	_, err := s.transitionWorkflow.transitionPlanWithTaskCancellation(
		ctx,
		orgID,
		planID,
		planTransitionSpec{
			action:          "cancel_plan",
			startLog:        "Canceling assessment plan",
			transitionLog:   "Plan canceled, canceling tasks",
			transitionError: "Failed to cancel plan",
			planSaveError:   "Failed to save canceled plan",
			taskSaveError:   "Failed to save canceled task",
			successLog:      "Plan canceled successfully",
		},
		s.lifecycle.Cancel,
	)
	return err
}
