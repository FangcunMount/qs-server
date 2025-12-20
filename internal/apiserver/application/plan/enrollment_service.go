package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
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
	// 1. 转换参数
	planID, err := toPlanID(dto.PlanID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	testeeID, err := toTesteeID(dto.TesteeID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	startDate, err := parseDate(dto.StartDate)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的开始日期: %v", err)
	}

	// 2. 调用领域服务加入计划
	tasks, err := s.enrollment.EnrollTestee(ctx, planID, testeeID, startDate)
	if err != nil {
		return nil, err
	}

	// 3. 持久化任务
	if len(tasks) > 0 {
		if err := s.taskRepo.SaveBatch(ctx, tasks); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
	}

	// 4. 发布领域事件
	// TODO: 发布 TesteeEnrolledInPlanEvent 事件

	return &EnrollmentResult{
		PlanID: dto.PlanID,
		Tasks:  toTaskResults(tasks),
	}, nil
}

// TerminateEnrollment 终止受试者的计划参与
func (s *enrollmentService) TerminateEnrollment(ctx context.Context, planID string, testeeID string) error {
	// 1. 转换参数
	planIDDomain, err := toPlanID(planID)
	if err != nil {
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	testeeIDDomain, err := toTesteeID(testeeID)
	if err != nil {
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的受试者ID: %v", err)
	}

	// 2. 调用领域服务终止参与
	canceledTasks, err := s.enrollment.TerminateEnrollment(ctx, planIDDomain, testeeIDDomain)
	if err != nil {
		return err
	}

	// 3. 持久化被取消的任务
	for _, task := range canceledTasks {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			// 记录错误但继续处理其他任务
			continue
		}

		// 发布任务事件
		events := task.Events()
		for _, evt := range events {
			if err := s.eventPublisher.Publish(ctx, evt); err != nil {
				// 记录错误但继续执行
			}
		}
		task.ClearEvents()
	}

	// TODO: 发布 TesteeTerminatedFromPlanEvent 事件

	return nil
}
