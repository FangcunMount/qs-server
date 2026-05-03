package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 计划生命周期服务实现
// 行为者：计划管理员
type lifecycleService struct {
	planRepo       plan.AssessmentPlanRepository
	taskRepo       plan.AssessmentTaskRepository
	scaleCatalog   ScaleCatalog
	validator      *plan.PlanValidator
	lifecycle      *plan.PlanLifecycle
	eventPublisher event.EventPublisher
}

type planTransitionSpec struct {
	action          string
	startLog        string
	transitionLog   string
	transitionError string
	planSaveError   string
	taskSaveError   string
	successLog      string
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
		planRepo:       planRepo,
		taskRepo:       taskRepo,
		scaleCatalog:   scaleCatalog,
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
		"scale_code", dto.ScaleCode,
		"schedule_type", dto.ScheduleType,
		"trigger_time", dto.TriggerTime,
		"interval", dto.Interval,
		"total_times", dto.TotalTimes,
		"fixed_dates", dto.FixedDates,
		"relative_weeks", dto.RelativeWeeks,
	)

	// 1. 验证 scale code 是否存在
	logger.L(ctx).Infow("CreatePlan validating scale_code",
		"action", "create_plan",
		"scale_code", dto.ScaleCode,
	)
	if s.scaleCatalog != nil {
		exists, err := s.scaleCatalog.ExistsByCode(ctx, dto.ScaleCode)
		if err != nil {
			logger.L(ctx).Errorw("CreatePlan scale validation error",
				"action", "create_plan",
				"scale_code", dto.ScaleCode,
				"error", err.Error(),
			)
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "验证量表编码失败: %s", dto.ScaleCode)
		}
		if !exists {
			logger.L(ctx).Errorw("CreatePlan scale not found",
				"action", "create_plan",
				"scale_code", dto.ScaleCode,
			)
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的量表编码: %s", dto.ScaleCode)
		}
		logger.L(ctx).Infow("CreatePlan scale_code validated",
			"action", "create_plan",
			"scale_code", dto.ScaleCode,
		)
	}

	logger.L(ctx).Infow("CreatePlan converting schedule_type",
		"action", "create_plan",
		"schedule_type", dto.ScheduleType,
	)
	scheduleType := toPlanScheduleType(dto.ScheduleType)
	logger.L(ctx).Infow("CreatePlan schedule_type converted",
		"action", "create_plan",
		"schedule_type_parsed", string(scheduleType),
	)

	triggerTime, err := plan.NormalizePlanTriggerTime(dto.TriggerTime)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan invalid trigger_time",
			"action", "create_plan",
			"trigger_time", dto.TriggerTime,
			"error", err.Error(),
		)
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "无效的触发时间: %s", dto.TriggerTime)
	}

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
		"scale_code", dto.ScaleCode,
		"schedule_type", string(scheduleType),
		"trigger_time", triggerTime,
		"interval", dto.Interval,
		"total_times", totalTimes,
		"fixed_dates_count", len(fixedDates),
		"relative_weeks_count", len(dto.RelativeWeeks),
	)
	if errs := s.validator.ValidateForCreation(dto.OrgID, dto.ScaleCode, scheduleType, triggerTime, dto.Interval, totalTimes, fixedDates, dto.RelativeWeeks); len(errs) > 0 {
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
		"trigger_time", triggerTime,
		"has_fixed_dates", len(fixedDates) > 0,
		"has_relative_weeks", len(dto.RelativeWeeks) > 0,
	)
	opts := []plan.PlanOption{plan.WithTriggerTime(triggerTime)}
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
		"scale_code", dto.ScaleCode,
		"schedule_type", string(scheduleType),
		"trigger_time", triggerTime,
		"interval", dto.Interval,
		"total_times", totalTimes,
	)
	p, err := plan.NewAssessmentPlan(dto.OrgID, dto.ScaleCode, scheduleType, dto.Interval, totalTimes, opts...)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan failed to create domain object",
			"action", "create_plan",
			"org_id", dto.OrgID,
			"scale_code", dto.ScaleCode,
			"schedule_type", string(scheduleType),
			"trigger_time", triggerTime,
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

	logger.L(ctx).Infow("CreatePlan completed successfully",
		"action", "create_plan",
		"plan_id", p.GetID().String(),
		"org_id", dto.OrgID,
	)

	return toPlanResult(p), nil
}

// PausePlan 暂停计划
func (s *lifecycleService) PausePlan(ctx context.Context, orgID int64, planID string) (*PlanResult, error) {
	return s.transitionPlanWithTaskCancellation(
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
	return s.transitionPlanWithTaskCancellation(
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
	_, err := s.transitionPlanWithTaskCancellation(
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

func (s *lifecycleService) transitionPlanWithTaskCancellation(
	ctx context.Context,
	orgID int64,
	planID string,
	spec planTransitionSpec,
	transition func(context.Context, *plan.AssessmentPlan) ([]*plan.AssessmentTask, error),
) (*PlanResult, error) {
	logger.L(ctx).Infow(spec.startLog,
		"action", spec.action,
		"org_id", orgID,
		"plan_id", planID,
	)

	planAggregate, err := loadPlanInOrg(ctx, s.planRepo, orgID, planID, spec.action)
	if err != nil {
		return nil, err
	}

	canceledTasks, err := transition(ctx, planAggregate)
	if err != nil {
		logger.L(ctx).Errorw(spec.transitionError,
			"action", spec.action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, err
	}

	logger.L(ctx).Infow(spec.transitionLog,
		"action", spec.action,
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
	)

	if err := s.planRepo.Save(ctx, planAggregate); err != nil {
		logger.L(ctx).Errorw(spec.planSaveError,
			"action", spec.action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	savedTaskCount := s.saveCanceledTasks(ctx, spec.action, planID, spec.taskSaveError, canceledTasks)

	logger.L(ctx).Infow(spec.successLog,
		"action", spec.action,
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(planAggregate), nil
}

func (s *lifecycleService) saveCanceledTasks(
	ctx context.Context,
	action string,
	planID string,
	taskSaveError string,
	tasks []*plan.AssessmentTask,
) int {
	savedTaskCount := 0
	for _, task := range tasks {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw(taskSaveError,
				"action", action,
				"plan_id", planID,
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			continue
		}
		savedTaskCount++

		eventing.PublishCollectedEvents(ctx, s.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
			logger.L(ctx).Errorw("Failed to publish task event",
				"action", action,
				"task_id", task.GetID().String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		})
	}
	return savedTaskCount
}
