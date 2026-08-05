package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// lifecycleService 计划生命周期服务实现
// 行为者：计划管理员
type lifecycleService struct {
	planRepo           plan.AssessmentPlanRepository
	taskRepo           plan.AssessmentTaskRepository
	tx                 apptransaction.Runner
	lifecycle          *plan.PlanLifecycle
	createWorkflow     *planCreateWorkflow
	transitionWorkflow *planTransitionWorkflow
}

func NewLifecycleServiceWithEnrollment(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	scaleCatalog ScaleCatalog,
	enrollments plan.PlanEnrollmentLifecycleRepository,
	tx apptransaction.Runner,
	eventPublisher event.EventPublisher,
) PlanLifecycleService {
	taskGenerator := plan.NewTaskGenerator()
	taskLifecycle := plan.NewTaskLifecycle()
	lifecycle := plan.NewPlanLifecycle(taskRepo, taskGenerator, taskLifecycle)

	return &lifecycleService{
		planRepo:           planRepo,
		taskRepo:           taskRepo,
		tx:                 tx,
		lifecycle:          lifecycle,
		createWorkflow:     newPlanCreateWorkflow(planRepo, scaleCatalog, plan.NewPlanValidator()),
		transitionWorkflow: newPlanTransitionWorkflow(planRepo, taskRepo, enrollments, tx, eventPublisher),
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

	parsedPlanID, err := plan.ParseAssessmentPlanID(planID)
	if err != nil {
		return nil, invalidArgumentErr("无效的计划ID: %v", err)
	}
	if s.tx == nil {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "恢复计划事务组件未配置")
	}
	var resumedPlan *plan.AssessmentPlan
	var resumeResult *plan.ResumeTasksResult
	actionAt := time.Now()
	resume := func(txCtx context.Context) error {
		p, loadErr := s.loadPlanForResume(txCtx, orgID, parsedPlanID)
		if loadErr != nil {
			return loadErr
		}
		tasks, loadErr := s.loadTasksForResume(txCtx, parsedPlanID)
		if loadErr != nil {
			return loadErr
		}
		result, lifecycleErr := s.lifecycle.ResumeWithTasksAt(txCtx, p, tasks, testeeStartDateMap, actionAt)
		if lifecycleErr != nil {
			return lifecycleErr
		}
		if saveErr := s.planRepo.Save(txCtx, p); saveErr != nil {
			return errors.WrapC(saveErr, errorCode.ErrDatabase, "保存计划失败")
		}
		for _, task := range result.TasksToSave {
			if saveErr := s.saveResumedTask(txCtx, task); saveErr != nil {
				if errors.IsCode(saveErr, errorCode.ErrConflict) {
					return saveErr
				}
				return errors.WrapC(saveErr, errorCode.ErrDatabase, "保存任务失败")
			}
		}
		resumedPlan, resumeResult = p, result
		return nil
	}
	err = s.tx.WithinTransaction(ctx, resume)
	if err != nil {
		logger.L(ctx).Errorw("Failed to resume plan", "action", "resume_plan", "plan_id", planID, "error", err.Error())
		return nil, err
	}
	savedTaskCount := len(resumeResult.TasksToSave)

	logger.L(ctx).Infow("Plan resumed successfully",
		"action", "resume_plan",
		"plan_id", planID,
		"tasks_to_save_count", len(resumeResult.TasksToSave),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(resumedPlan), nil
}

type resumeTaskRevisionRepository interface {
	SaveRescheduled(context.Context, *plan.AssessmentTask, uint32) error
}

func (s *lifecycleService) saveResumedTask(ctx context.Context, task *plan.AssessmentTask) error {
	if task.GetID().IsZero() || task.GetScheduleRevision() <= 1 {
		return s.taskRepo.Save(ctx, task)
	}
	if repository, ok := s.taskRepo.(resumeTaskRevisionRepository); ok {
		return repository.SaveRescheduled(ctx, task, task.GetScheduleRevision()-1)
	}
	return s.taskRepo.Save(ctx, task)
}

type resumePlanLockingRepository interface {
	FindByIDForUpdate(context.Context, plan.AssessmentPlanID) (*plan.AssessmentPlan, error)
}

type resumeTaskLockingRepository interface {
	FindByPlanIDForUpdate(context.Context, plan.AssessmentPlanID) ([]*plan.AssessmentTask, error)
}

func (s *lifecycleService) loadPlanForResume(ctx context.Context, orgID int64, id plan.AssessmentPlanID) (*plan.AssessmentPlan, error) {
	if locking, ok := s.planRepo.(resumePlanLockingRepository); ok {
		p, err := locking.FindByIDForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		if p.GetOrgID() != orgID {
			return nil, errors.WithCode(errorCode.ErrPageNotFound, "plan not found")
		}
		return p, nil
	}
	return loadPlanInOrg(ctx, s.planRepo, orgID, id.String(), "resume_plan")
}

func (s *lifecycleService) loadTasksForResume(ctx context.Context, id plan.AssessmentPlanID) ([]*plan.AssessmentTask, error) {
	if locking, ok := s.taskRepo.(resumeTaskLockingRepository); ok {
		return locking.FindByPlanIDForUpdate(ctx, id)
	}
	return s.taskRepo.FindByPlanID(ctx, id)
}

// FinishPlan 手动结束计划
func (s *lifecycleService) FinishPlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.transitionWorkflow.transitionPlanWithTaskCancellation(
		ctx,
		orgID,
		planID,
		planTransitionSpec{
			action:           "finish_plan",
			startLog:         "Finishing assessment plan",
			transitionLog:    "Plan finished, canceling outstanding tasks",
			transitionError:  "Failed to finish plan",
			planSaveError:    "Failed to save finished plan",
			taskSaveError:    "Failed to save canceled task while finishing plan",
			successLog:       "Plan finished successfully",
			enrollmentAction: "close",
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
			action:           "cancel_plan",
			startLog:         "Canceling assessment plan",
			transitionLog:    "Plan canceled, canceling tasks",
			transitionError:  "Failed to cancel plan",
			planSaveError:    "Failed to save canceled plan",
			taskSaveError:    "Failed to save canceled task",
			successLog:       "Plan canceled successfully",
			enrollmentAction: "terminate",
		},
		s.lifecycle.Cancel,
	)
	return err
}
