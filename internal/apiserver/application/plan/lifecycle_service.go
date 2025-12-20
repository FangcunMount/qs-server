package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 计划生命周期服务实现
// 行为者：计划管理员
type lifecycleService struct {
	planRepo       plan.AssessmentPlanRepository
	taskRepo       plan.AssessmentTaskRepository
	validator      *plan.PlanValidator
	lifecycle      *plan.PlanLifecycle
	eventPublisher event.EventPublisher
}

// NewLifecycleService 创建计划生命周期服务
func NewLifecycleService(
	planRepo plan.AssessmentPlanRepository,
	taskRepo plan.AssessmentTaskRepository,
	eventPublisher event.EventPublisher,
) PlanLifecycleService {
	taskGenerator := plan.NewTaskGenerator()
	taskLifecycle := plan.NewTaskLifecycle()
	lifecycle := plan.NewPlanLifecycle(taskRepo, taskGenerator, taskLifecycle)

	return &lifecycleService{
		planRepo:       planRepo,
		taskRepo:       taskRepo,
		validator:      plan.NewPlanValidator(),
		lifecycle:      lifecycle,
		eventPublisher: eventPublisher,
	}
}

// CreatePlan 创建测评计划模板
func (s *lifecycleService) CreatePlan(ctx context.Context, dto CreatePlanDTO) (*PlanResult, error) {
	// 1. 转换参数
	scaleID, err := toScaleID(dto.ScaleID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的量表ID: %v", err)
	}

	scheduleType := toPlanScheduleType(dto.ScheduleType)

	// 转换固定日期列表
	var fixedDates []time.Time
	if len(dto.FixedDates) > 0 {
		fixedDates = make([]time.Time, 0, len(dto.FixedDates))
		for _, dateStr := range dto.FixedDates {
			date, err := parseDate(dateStr)
			if err != nil {
				return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的日期格式: %s", dateStr)
			}
			fixedDates = append(fixedDates, date)
		}
	}

	// 2. 验证参数
	if errs := s.validator.ValidateForCreation(dto.OrgID, scaleID, scheduleType, dto.Interval, dto.TotalTimes, fixedDates, dto.RelativeWeeks); len(errs) > 0 {
		return nil, plan.ToError(errs)
	}

	// 3. 创建计划选项
	var opts []plan.PlanOption
	if len(fixedDates) > 0 {
		opts = append(opts, plan.WithFixedDates(fixedDates))
	}
	if len(dto.RelativeWeeks) > 0 {
		opts = append(opts, plan.WithRelativeWeeks(dto.RelativeWeeks))
	}

	// 4. 创建计划领域对象
	p, err := plan.NewAssessmentPlan(dto.OrgID, scaleID, scheduleType, dto.Interval, dto.TotalTimes, opts...)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建计划失败")
	}

	// 5. 持久化
	if err := s.planRepo.Save(ctx, p); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 6. 发布领域事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			// 记录错误但继续执行
		}
	}
	p.ClearEvents()

	return toPlanResult(p), nil
}

// PausePlan 暂停计划
func (s *lifecycleService) PausePlan(ctx context.Context, planID string) (*PlanResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务暂停计划
	canceledTasks, err := s.lifecycle.Pause(ctx, p)
	if err != nil {
		return nil, err
	}

	// 4. 持久化计划
	if err := s.planRepo.Save(ctx, p); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 持久化被取消的任务
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

	// 6. 发布计划事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			// 记录错误但继续执行
		}
	}
	p.ClearEvents()

	return toPlanResult(p), nil
}

// ResumePlan 恢复计划
func (s *lifecycleService) ResumePlan(ctx context.Context, planID string, testeeStartDates map[string]string) (*PlanResult, error) {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
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

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务恢复计划
	newTasks, err := s.lifecycle.Resume(ctx, p, testeeStartDateMap)
	if err != nil {
		return nil, err
	}

	// 4. 持久化计划
	if err := s.planRepo.Save(ctx, p); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 持久化新生成的任务
	if len(newTasks) > 0 {
		if err := s.taskRepo.SaveBatch(ctx, newTasks); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
	}

	// 6. 发布计划事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			// 记录错误但继续执行
		}
	}
	p.ClearEvents()

	return toPlanResult(p), nil
}

// CancelPlan 取消计划
func (s *lifecycleService) CancelPlan(ctx context.Context, planID string) error {
	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		return errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务取消计划
	if err := s.lifecycle.Cancel(ctx, p); err != nil {
		return err
	}

	// 4. 持久化
	if err := s.planRepo.Save(ctx, p); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 发布领域事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			// 记录错误但继续执行
		}
	}
	p.ClearEvents()

	return nil
}
