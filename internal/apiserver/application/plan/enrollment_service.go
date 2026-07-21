package plan

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// enrollmentService 受试者加入计划服务实现
// 行为者：受试者管理服务
type enrollmentService struct {
	planRepo        plan.AssessmentPlanRepository
	taskRepo        plan.AssessmentTaskRepository
	enrollmentTasks plan.EnrollmentTaskRepository
	enrollmentRepo  plan.EnrollmentRepository
	txRunner        apptransaction.Runner
	taskGenerator   *plan.TaskGenerator
	validator       *plan.PlanValidator
	eventPublisher  event.EventPublisher
}

// NewEnrollmentService 创建受试者加入计划服务
func NewEnrollmentService(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	enrollmentRepo plan.EnrollmentRepository,
	txRunner apptransaction.Runner,
	eventPublisher event.EventPublisher,
) PlanEnrollmentService {
	enrollmentTasks, ok := taskRepo.(plan.EnrollmentTaskRepository)
	if !ok {
		panic("plan task repository must implement EnrollmentTaskRepository")
	}
	return &enrollmentService{
		planRepo:        planRepo,
		taskRepo:        taskRepo,
		enrollmentTasks: enrollmentTasks,
		enrollmentRepo:  enrollmentRepo,
		txRunner:        txRunner,
		taskGenerator:   plan.NewTaskGenerator(),
		validator:       plan.NewPlanValidator(),
		eventPublisher:  eventPublisher,
	}
}

// EnrollTestee 将受试者加入计划
func (s *enrollmentService) EnrollTestee(ctx context.Context, dto EnrollTesteeDTO) (*EnrollmentResult, error) {
	logger.L(ctx).Infow("Enrolling testee to plan",
		"action", "enroll_testee",
		"org_id", dto.OrgID,
		"plan_id", dto.PlanID,
		"testee_id", dto.TesteeID,
		"start_date", dto.StartDate,
	)

	// 1. 转换参数
	planID, err := toPlanID(dto.PlanID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", "enroll_testee",
			"plan_id", dto.PlanID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	testeeID, err := toTesteeID(dto.TesteeID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid testee ID",
			"action", "enroll_testee",
			"testee_id", dto.TesteeID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	startDate, err := parseDate(dto.StartDate)
	if err != nil {
		logger.L(ctx).Errorw("Invalid start date",
			"action", "enroll_testee",
			"start_date", dto.StartDate,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的开始日期: %v", err)
	}

	var result *EnrollmentResult
	err = s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		planAggregate, err := s.loadPlanInOrg(txCtx, dto.OrgID, planID, "enroll_testee")
		if err != nil {
			return err
		}

		active, err := s.enrollmentRepo.FindActive(txCtx, dto.OrgID, planID, testeeID)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询活动参与轮次失败")
		}
		if active != nil {
			if !sameBusinessDate(active.StartDate(), startDate) {
				return errors.WithCode(errorCode.ErrInvalidArgument, "受试者已加入此计划，且开始日期与活动轮次不一致")
			}
			tasks, err := s.enrollmentTasks.FindByEnrollmentID(txCtx, active.ID())
			if err != nil {
				return errors.WrapC(err, errorCode.ErrDatabase, "查询参与任务失败")
			}
			result = &EnrollmentResult{PlanID: dto.PlanID, EnrollmentID: active.ID().String(), Round: active.Round(), Tasks: toTaskResults(tasks), Idempotent: true}
			return nil
		}

		if validationErrors := s.validator.ValidateForEnrollment(planAggregate, testeeID, startDate); len(validationErrors) > 0 {
			return plan.ToError(validationErrors)
		}
		latest, err := s.enrollmentRepo.FindLatest(txCtx, dto.OrgID, planID, testeeID)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询参与轮次失败")
		}
		round := uint32(1)
		if latest != nil {
			round = latest.Round() + 1
		}
		enrollment := plan.NewEnrollment(dto.OrgID, planID, testeeID, round, startDate, time.Now())
		tasks := s.taskGenerator.GenerateTasks(planAggregate, testeeID, startDate)
		if len(tasks) == 0 {
			return errors.WithCode(errorCode.ErrInvalidArgument, "未能生成任何任务")
		}
		for _, task := range tasks {
			task.AssignEnrollment(enrollment.ID())
		}
		if err := s.enrollmentRepo.Save(txCtx, enrollment); err != nil {
			return err
		}
		if err := s.taskRepo.SaveBatch(txCtx, tasks); err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
		result = &EnrollmentResult{
			PlanID: dto.PlanID, EnrollmentID: enrollment.ID().String(), Round: enrollment.Round(),
			Tasks: toTaskResults(tasks), CreatedTaskCount: len(tasks),
		}
		return nil
	})
	if err != nil {
		if stderrors.Is(err, plan.ErrActiveEnrollmentExists) {
			active, lookupErr := s.enrollmentRepo.FindActive(ctx, dto.OrgID, planID, testeeID)
			if lookupErr == nil && active != nil && sameBusinessDate(active.StartDate(), startDate) {
				tasks, taskErr := s.enrollmentTasks.FindByEnrollmentID(ctx, active.ID())
				if taskErr == nil {
					return &EnrollmentResult{PlanID: dto.PlanID, EnrollmentID: active.ID().String(), Round: active.Round(), Tasks: toTaskResults(tasks), Idempotent: true}, nil
				}
			}
		}
		logger.L(ctx).Errorw("Failed to enroll testee",
			"action", "enroll_testee",
			"plan_id", dto.PlanID,
			"testee_id", dto.TesteeID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow("Testee enrolled successfully",
		"action", "enroll_testee",
		"plan_id", dto.PlanID,
		"testee_id", dto.TesteeID,
		"enrollment_id", result.EnrollmentID,
		"round", result.Round,
		"tasks_count", len(result.Tasks),
		"idempotent", result.Idempotent,
	)

	return result, nil
}

// TerminateEnrollment 终止受试者的计划参与
func (s *enrollmentService) TerminateEnrollment(ctx context.Context, orgID int64, planID string, testeeID string) error {
	logger.L(ctx).Infow("Terminating testee enrollment",
		"action", "terminate_enrollment",
		"org_id", orgID,
		"plan_id", planID,
		"testee_id", testeeID,
	)

	// 1. 转换参数
	planIDDomain, err := toPlanID(planID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", "terminate_enrollment",
			"plan_id", planID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid testee ID",
			"action", "terminate_enrollment",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	var canceledTasks []*plan.AssessmentTask
	err = s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := s.loadPlanInOrg(txCtx, orgID, planIDDomain, "terminate_enrollment"); err != nil {
			return err
		}
		enrollment, err := s.enrollmentRepo.FindActive(txCtx, orgID, planIDDomain, testeeIDDomain)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询活动参与轮次失败")
		}
		if enrollment == nil {
			return nil
		}
		tasks, err := s.enrollmentTasks.FindByEnrollmentID(txCtx, enrollment.ID())
		if err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "查询参与任务失败")
		}
		lifecycle := plan.NewTaskLifecycle()
		for _, task := range tasks {
			if task.IsTerminal() {
				continue
			}
			if err := lifecycle.Cancel(txCtx, task); err != nil {
				return err
			}
			if err := s.taskRepo.Save(txCtx, task); err != nil {
				return errors.WrapC(err, errorCode.ErrDatabase, "保存取消任务失败")
			}
			canceledTasks = append(canceledTasks, task)
		}
		enrollment.Terminate(time.Now(), "terminated_by_command")
		if err := s.enrollmentRepo.Save(txCtx, enrollment); err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "保存终止参与轮次失败")
		}
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("Failed to terminate enrollment",
			"action", "terminate_enrollment",
			"plan_id", planID,
			"testee_id", testeeID,
			"error", err.Error(),
		)
		return err
	}

	logger.L(ctx).Infow("Enrollment terminated, canceling tasks",
		"action", "terminate_enrollment",
		"plan_id", planID,
		"testee_id", testeeID,
		"canceled_tasks_count", len(canceledTasks),
	)

	for _, task := range canceledTasks {
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", "terminate_enrollment",
				"task_id", task.GetID().String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		})
	}

	logger.L(ctx).Infow("Enrollment terminated successfully",
		"action", "terminate_enrollment",
		"plan_id", planID,
		"testee_id", testeeID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", len(canceledTasks),
	)

	return nil
}

func sameBusinessDate(left, right time.Time) bool {
	ly, lm, ld := left.Date()
	ry, rm, rd := right.Date()
	return ly == ry && lm == rm && ld == rd
}

func (s *enrollmentService) loadPlanInOrg(ctx context.Context, orgID int64, planID plan.AssessmentPlanID, action string) (*plan.AssessmentPlan, error) {
	p, err := s.planRepo.FindByID(ctx, planID)
	if err != nil {
		logger.L(ctx).Errorw("Plan not found",
			"action", action,
			"plan_id", planID.String(),
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	if p.GetOrgID() != orgID {
		logger.L(ctx).Warnw("Plan access denied due to org scope mismatch",
			"action", action,
			"plan_id", planID.String(),
			"request_org_id", orgID,
			"resource_org_id", p.GetOrgID(),
		)
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "计划不属于当前机构")
	}

	return p, nil
}
