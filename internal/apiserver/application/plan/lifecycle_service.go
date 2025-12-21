package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	logger.L(ctx).Infow("CreatePlan service started",
		"action", "create_plan",
		"org_id", dto.OrgID,
		"scale_id", dto.ScaleID,
		"schedule_type", dto.ScheduleType,
		"interval", dto.Interval,
		"total_times", dto.TotalTimes,
		"fixed_dates", dto.FixedDates,
		"relative_weeks", dto.RelativeWeeks,
	)

	// 1. 转换参数
	logger.L(ctx).Infow("CreatePlan converting scale_id",
		"action", "create_plan",
		"scale_id", dto.ScaleID,
	)
	scaleID, err := toScaleID(dto.ScaleID)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan invalid scale ID",
			"action", "create_plan",
			"scale_id", dto.ScaleID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的量表ID: %v", err)
	}
	logger.L(ctx).Infow("CreatePlan scale_id converted",
		"action", "create_plan",
		"scale_id_parsed", scaleID.String(),
	)

	logger.L(ctx).Infow("CreatePlan converting schedule_type",
		"action", "create_plan",
		"schedule_type", dto.ScheduleType,
	)
	scheduleType := toPlanScheduleType(dto.ScheduleType)
	logger.L(ctx).Infow("CreatePlan schedule_type converted",
		"action", "create_plan",
		"schedule_type_parsed", string(scheduleType),
	)

	// 转换固定日期列表
	var fixedDates []time.Time
	if len(dto.FixedDates) > 0 {
		logger.L(ctx).Infow("CreatePlan parsing fixed_dates",
			"action", "create_plan",
			"fixed_dates_count", len(dto.FixedDates),
			"fixed_dates", dto.FixedDates,
		)
		fixedDates = make([]time.Time, 0, len(dto.FixedDates))
		for i, dateStr := range dto.FixedDates {
			logger.L(ctx).Infow("CreatePlan parsing fixed_date",
				"action", "create_plan",
				"index", i,
				"date_str", dateStr,
			)
			date, err := parseDate(dateStr)
			if err != nil {
				logger.L(ctx).Errorw("CreatePlan invalid date format",
					"action", "create_plan",
					"index", i,
					"date_str", dateStr,
					"error", err.Error(),
				)
				return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的日期格式: %s", dateStr)
			}
			fixedDates = append(fixedDates, date)
			logger.L(ctx).Infow("CreatePlan fixed_date parsed",
				"action", "create_plan",
				"index", i,
				"date", date.Format("2006-01-02"),
			)
		}
		logger.L(ctx).Infow("CreatePlan all fixed_dates parsed",
			"action", "create_plan",
			"fixed_dates_count", len(fixedDates),
		)
	}

	// 2. 根据 schedule_type 确定 totalTimes
	// 对于 custom 和 fixed_date 类型，totalTimes 应该从对应的数组长度推导
	totalTimes := dto.TotalTimes
	logger.L(ctx).Infow("CreatePlan calculating total_times",
		"action", "create_plan",
		"initial_total_times", totalTimes,
		"schedule_type", scheduleType,
	)
	switch scheduleType {
	case plan.PlanScheduleCustom:
		if len(dto.RelativeWeeks) > 0 {
			totalTimes = len(dto.RelativeWeeks)
			logger.L(ctx).Infow("CreatePlan total_times from relative_weeks",
				"action", "create_plan",
				"total_times", totalTimes,
				"relative_weeks_count", len(dto.RelativeWeeks),
			)
		}
	case plan.PlanScheduleFixedDate:
		if len(fixedDates) > 0 {
			totalTimes = len(fixedDates)
			logger.L(ctx).Infow("CreatePlan total_times from fixed_dates",
				"action", "create_plan",
				"total_times", totalTimes,
				"fixed_dates_count", len(fixedDates),
			)
		}
	}
	logger.L(ctx).Infow("CreatePlan total_times calculated",
		"action", "create_plan",
		"final_total_times", totalTimes,
	)

	// 3. 验证参数（使用计算后的 totalTimes）
	logger.L(ctx).Infow("CreatePlan validating parameters",
		"action", "create_plan",
		"org_id", dto.OrgID,
		"scale_id", scaleID.String(),
		"schedule_type", string(scheduleType),
		"interval", dto.Interval,
		"total_times", totalTimes,
		"fixed_dates_count", len(fixedDates),
		"relative_weeks_count", len(dto.RelativeWeeks),
	)
	if errs := s.validator.ValidateForCreation(dto.OrgID, scaleID, scheduleType, dto.Interval, totalTimes, fixedDates, dto.RelativeWeeks); len(errs) > 0 {
		logger.L(ctx).Errorw("CreatePlan validation failed",
			"action", "create_plan",
			"org_id", dto.OrgID,
			"validation_errors", errs,
			"errors_count", len(errs),
		)
		for i, err := range errs {
			logger.L(ctx).Errorw("CreatePlan validation error detail",
				"action", "create_plan",
				"error_index", i,
				"field", err.Field,
				"message", err.Message,
			)
		}
		return nil, plan.ToError(errs)
	}
	logger.L(ctx).Infow("CreatePlan validation passed",
		"action", "create_plan",
	)

	// 4. 创建计划选项
	logger.L(ctx).Infow("CreatePlan building plan options",
		"action", "create_plan",
		"has_fixed_dates", len(fixedDates) > 0,
		"has_relative_weeks", len(dto.RelativeWeeks) > 0,
	)
	var opts []plan.PlanOption
	if len(fixedDates) > 0 {
		opts = append(opts, plan.WithFixedDates(fixedDates))
		logger.L(ctx).Infow("CreatePlan added fixed_dates option",
			"action", "create_plan",
			"fixed_dates_count", len(fixedDates),
		)
	}
	if len(dto.RelativeWeeks) > 0 {
		opts = append(opts, plan.WithRelativeWeeks(dto.RelativeWeeks))
		logger.L(ctx).Infow("CreatePlan added relative_weeks option",
			"action", "create_plan",
			"relative_weeks_count", len(dto.RelativeWeeks),
		)
	}
	logger.L(ctx).Infow("CreatePlan plan options built",
		"action", "create_plan",
		"options_count", len(opts),
	)

	// 5. 创建计划领域对象
	logger.L(ctx).Infow("CreatePlan creating domain object",
		"action", "create_plan",
		"org_id", dto.OrgID,
		"scale_id", scaleID.String(),
		"schedule_type", string(scheduleType),
		"interval", dto.Interval,
		"total_times", totalTimes,
	)
	p, err := plan.NewAssessmentPlan(dto.OrgID, scaleID, scheduleType, dto.Interval, totalTimes, opts...)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan failed to create domain object",
			"action", "create_plan",
			"org_id", dto.OrgID,
			"scale_id", scaleID.String(),
			"schedule_type", string(scheduleType),
			"interval", dto.Interval,
			"total_times", totalTimes,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建计划失败")
	}
	logger.L(ctx).Infow("CreatePlan domain object created",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
	)

	// 6. 持久化
	logger.L(ctx).Infow("CreatePlan saving to repository",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
	)
	if err := s.planRepo.Save(ctx, p); err != nil {
		logger.L(ctx).Errorw("CreatePlan failed to save plan",
			"action", "create_plan",
			"plan_id", p.GetID().String(),
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}
	logger.L(ctx).Infow("CreatePlan plan saved",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
	)

	// 7. 发布领域事件
	events := p.Events()
	eventCount := len(events)
	logger.L(ctx).Infow("CreatePlan publishing events",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
		"events_count", eventCount,
	)
	for i, evt := range events {
		logger.L(ctx).Infow("CreatePlan publishing event",
			"action", "create_plan",
			"plan_id", p.GetID().String(),
			"event_index", i,
			"event_type", evt.EventType(),
		)
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("CreatePlan failed to publish event",
				"action", "create_plan",
				"plan_id", p.GetID().String(),
				"event_index", i,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		} else {
			logger.L(ctx).Infow("CreatePlan event published",
				"action", "create_plan",
				"plan_id", p.GetID().String(),
				"event_index", i,
				"event_type", evt.EventType(),
			)
		}
	}
	p.ClearEvents()

	logger.L(ctx).Infow("CreatePlan completed successfully",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
		"org_id", dto.OrgID,
		"events_published", eventCount,
	)

	return toPlanResult(p), nil
}

// PausePlan 暂停计划
func (s *lifecycleService) PausePlan(ctx context.Context, planID string) (*PlanResult, error) {
	logger.L(ctx).Infow("Pausing assessment plan",
		"action", "pause_plan",
		"plan_id", planID,
	)

	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", "pause_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Plan not found",
			"action", "pause_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务暂停计划
	canceledTasks, err := s.lifecycle.Pause(ctx, p)
	if err != nil {
		logger.L(ctx).Errorw("Failed to pause plan",
			"action", "pause_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow("Plan paused, canceling tasks",
		"action", "pause_plan",
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
	)

	// 4. 持久化计划
	if err := s.planRepo.Save(ctx, p); err != nil {
		logger.L(ctx).Errorw("Failed to save paused plan",
			"action", "pause_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 持久化被取消的任务
	savedTaskCount := 0
	for _, task := range canceledTasks {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw("Failed to save canceled task",
				"action", "pause_plan",
				"plan_id", planID,
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
					"action", "pause_plan",
					"task_id", task.GetID().String(),
					"event_type", evt.EventType(),
					"error", err.Error(),
				)
			}
		}
		task.ClearEvents()
	}

	// 6. 发布计划事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish plan event",
				"action", "pause_plan",
				"plan_id", planID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	p.ClearEvents()

	logger.L(ctx).Infow("Plan paused successfully",
		"action", "pause_plan",
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(p), nil
}

// ResumePlan 恢复计划
func (s *lifecycleService) ResumePlan(ctx context.Context, planID string, testeeStartDates map[string]string) (*PlanResult, error) {
	logger.L(ctx).Infow("Resuming assessment plan",
		"action", "resume_plan",
		"plan_id", planID,
		"testee_count", len(testeeStartDates),
	)

	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
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
		logger.L(ctx).Errorw("Plan not found",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务恢复计划
	newTasks, err := s.lifecycle.Resume(ctx, p, testeeStartDateMap)
	if err != nil {
		logger.L(ctx).Errorw("Failed to resume plan",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow("Plan resumed, generating new tasks",
		"action", "resume_plan",
		"plan_id", planID,
		"new_tasks_count", len(newTasks),
	)

	// 4. 持久化计划
	if err := s.planRepo.Save(ctx, p); err != nil {
		logger.L(ctx).Errorw("Failed to save resumed plan",
			"action", "resume_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 持久化新生成的任务
	if len(newTasks) > 0 {
		if err := s.taskRepo.SaveBatch(ctx, newTasks); err != nil {
			logger.L(ctx).Errorw("Failed to save new tasks",
				"action", "resume_plan",
				"plan_id", planID,
				"tasks_count", len(newTasks),
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存任务失败")
		}
	}

	// 6. 发布计划事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish plan event",
				"action", "resume_plan",
				"plan_id", planID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	p.ClearEvents()

	logger.L(ctx).Infow("Plan resumed successfully",
		"action", "resume_plan",
		"plan_id", planID,
		"new_tasks_count", len(newTasks),
	)

	return toPlanResult(p), nil
}

// CancelPlan 取消计划
func (s *lifecycleService) CancelPlan(ctx context.Context, planID string) error {
	logger.L(ctx).Infow("Canceling assessment plan",
		"action", "cancel_plan",
		"plan_id", planID,
	)

	// 1. 转换参数
	id, err := toPlanID(planID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid plan ID",
			"action", "cancel_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的计划ID: %v", err)
	}

	// 2. 查询计划
	p, err := s.planRepo.FindByID(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw("Plan not found",
			"action", "cancel_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
	}

	// 3. 调用领域服务取消计划
	if err := s.lifecycle.Cancel(ctx, p); err != nil {
		logger.L(ctx).Errorw("Failed to cancel plan",
			"action", "cancel_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return err
	}

	// 4. 持久化
	if err := s.planRepo.Save(ctx, p); err != nil {
		logger.L(ctx).Errorw("Failed to save canceled plan",
			"action", "cancel_plan",
			"plan_id", planID,
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	// 5. 发布领域事件
	events := p.Events()
	for _, evt := range events {
		if err := s.eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish plan event",
				"action", "cancel_plan",
				"plan_id", planID,
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	p.ClearEvents()

	logger.L(ctx).Infow("Plan canceled successfully",
		"action", "cancel_plan",
		"plan_id", planID,
	)

	return nil
}
