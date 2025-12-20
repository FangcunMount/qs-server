package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// enrollmentService 受试者加入计划服务实现
// 行为者：受试者管理服务
type enrollmentService struct {
	planRepo       plan.AssessmentPlanRepository
	taskRepo       plan.AssessmentTaskRepository
	enrollment     *plan.PlanEnrollment
	eventPublisher event.EventPublisher
}

// NewEnrollmentService 创建受试者加入计划服务
func NewEnrollmentService(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	eventPublisher event.EventPublisher,
) PlanEnrollmentService {
	taskGenerator := plan.NewTaskGenerator()
	validator := plan.NewPlanValidator()
	enrollment := plan.NewPlanEnrollment(planRepo, taskRepo, taskGenerator, validator)

	return &enrollmentService{
		planRepo:       planRepo,
		taskRepo:       taskRepo,
		enrollment:     enrollment,
		eventPublisher: eventPublisher,
	}
}

// EnrollTestee 将受试者加入计划
func (s *enrollmentService) EnrollTestee(ctx context.Context, dto EnrollTesteeDTO) (*EnrollmentResult, error) {
	logger.L(ctx).Infow("Enrolling testee to plan",
		"action", "enroll_testee",
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

	// 2. 调用领域服务加入计划
	tasks, err := s.enrollment.EnrollTestee(ctx, planID, testeeID, startDate)
	if err != nil {
		logger.L(ctx).Errorw("Failed to enroll testee",
			"action", "enroll_testee",
			"plan_id", dto.PlanID,
			"testee_id", dto.TesteeID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow("Tasks generated for enrollment",
		"action", "enroll_testee",
		"plan_id", dto.PlanID,
		"testee_id", dto.TesteeID,
		"tasks_count", len(tasks),
	)

	// 3. 持久化任务
	if len(tasks) > 0 {
		if err := s.taskRepo.SaveBatch(ctx, tasks); err != nil {
			logger.L(ctx).Errorw("Failed to save tasks",
				"action", "enroll_testee",
				"plan_id", dto.PlanID,
				"testee_id", dto.TesteeID,
				"tasks_count", len(tasks),
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
	}

	// 4. 发布领域事件
	// TODO: 发布 TesteeEnrolledInPlanEvent 事件

	logger.L(ctx).Infow("Testee enrolled successfully",
		"action", "enroll_testee",
		"plan_id", dto.PlanID,
		"testee_id", dto.TesteeID,
		"tasks_count", len(tasks),
	)

	return &EnrollmentResult{
		PlanID: dto.PlanID,
		Tasks:  toTaskResults(tasks),
	}, nil
}

// TerminateEnrollment 终止受试者的计划参与
func (s *enrollmentService) TerminateEnrollment(ctx context.Context, planID string, testeeID string) error {
	logger.L(ctx).Infow("Terminating testee enrollment",
		"action", "terminate_enrollment",
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

	// 2. 调用领域服务终止参与
	canceledTasks, err := s.enrollment.TerminateEnrollment(ctx, planIDDomain, testeeIDDomain)
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

	// 3. 持久化被取消的任务
	savedTaskCount := 0
	for _, task := range canceledTasks {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to save canceled task",
				"action", "terminate_enrollment",
				"plan_id", planID,
				"testee_id", testeeID,
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			continue
		}
		savedTaskCount++

		// 发布任务事件
		events := task.Events()
		for _, evt := range events {
			if err := s.eventPublisher.Publish(ctx, evt); err != nil {
				logger.L(ctx).Errorw("Failed to publish task event",
					"action", "terminate_enrollment",
					"task_id", task.GetID().String(),
					"event_type", evt.EventType(),
					"error", err.Error(),
				)
			}
		}
		task.ClearEvents()
	}

	// TODO: 发布 TesteeTerminatedFromPlanEvent 事件

	logger.L(ctx).Infow("Enrollment terminated successfully",
		"action", "terminate_enrollment",
		"plan_id", planID,
		"testee_id", testeeID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", savedTaskCount,
	)

	return nil
}
