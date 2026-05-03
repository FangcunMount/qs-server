package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type planCreateWorkflow struct {
	planRepo     domainPlan.AssessmentPlanRepository
	scaleCatalog ScaleCatalog
	validator    *domainPlan.PlanValidator
}

type planCreateCommand struct {
	scheduleType domainPlan.PlanScheduleType
	triggerTime  string
	fixedDates   []time.Time
	totalTimes   int
	options      []domainPlan.PlanOption
}

func newPlanCreateWorkflow(
	planRepo domainPlan.AssessmentPlanRepository,
	scaleCatalog ScaleCatalog,
	validator *domainPlan.PlanValidator,
) *planCreateWorkflow {
	return &planCreateWorkflow{
		planRepo:     planRepo,
		scaleCatalog: scaleCatalog,
		validator:    validator,
	}
}

func (w *planCreateWorkflow) create(ctx context.Context, dto CreatePlanDTO) (*domainPlan.AssessmentPlan, error) {
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

	if err := w.validateScale(ctx, dto.ScaleCode); err != nil {
		return nil, err
	}
	command, err := w.assembleCommand(ctx, dto)
	if err != nil {
		return nil, err
	}
	if err := w.validateDomain(ctx, dto, command); err != nil {
		return nil, err
	}
	planAggregate, err := w.createAggregate(ctx, dto, command)
	if err != nil {
		return nil, err
	}
	if err := w.save(ctx, planAggregate); err != nil {
		return nil, err
	}

	logger.L(ctx).Infow("CreatePlan completed successfully",
		"action", "create_plan",
		"plan_id", planAggregate.GetID().String(),
		"org_id", dto.OrgID,
	)
	return planAggregate, nil
}

func (w *planCreateWorkflow) validateScale(ctx context.Context, scaleCode string) error {
	logger.L(ctx).Infow("CreatePlan validating scale_code",
		"action", "create_plan",
		"scale_code", scaleCode,
	)
	if w == nil || w.scaleCatalog == nil {
		return nil
	}
	exists, err := w.scaleCatalog.ExistsByCode(ctx, scaleCode)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan scale validation error",
			"action", "create_plan",
			"scale_code", scaleCode,
			"error", err.Error(),
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "验证量表编码失败: %s", scaleCode)
	}
	if !exists {
		logger.L(ctx).Errorw("CreatePlan scale not found",
			"action", "create_plan",
			"scale_code", scaleCode,
		)
		return errors.WithCode(errorCode.ErrInvalidArgument, "无效的量表编码: %s", scaleCode)
	}
	logger.L(ctx).Infow("CreatePlan scale_code validated",
		"action", "create_plan",
		"scale_code", scaleCode,
	)
	return nil
}

func (w *planCreateWorkflow) assembleCommand(ctx context.Context, dto CreatePlanDTO) (planCreateCommand, error) {
	logger.L(ctx).Infow("CreatePlan converting schedule_type",
		"action", "create_plan",
		"schedule_type", dto.ScheduleType,
	)
	scheduleType := toPlanScheduleType(dto.ScheduleType)
	logger.L(ctx).Infow("CreatePlan schedule_type converted",
		"action", "create_plan",
		"schedule_type_parsed", string(scheduleType),
	)

	triggerTime, err := domainPlan.NormalizePlanTriggerTime(dto.TriggerTime)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan invalid trigger_time",
			"action", "create_plan",
			"trigger_time", dto.TriggerTime,
			"error", err.Error(),
		)
		return planCreateCommand{}, errors.WithCode(errorCode.ErrInvalidArgument, "无效的触发时间: %s", dto.TriggerTime)
	}

	fixedDates, err := parseFixedDates(ctx, dto.FixedDates)
	if err != nil {
		return planCreateCommand{}, err
	}

	totalTimes := derivePlanTotalTimes(ctx, scheduleType, dto.TotalTimes, fixedDates, dto.RelativeWeeks)
	options := buildPlanOptions(ctx, triggerTime, fixedDates, dto.RelativeWeeks)
	return planCreateCommand{
		scheduleType: scheduleType,
		triggerTime:  triggerTime,
		fixedDates:   fixedDates,
		totalTimes:   totalTimes,
		options:      options,
	}, nil
}

func (w *planCreateWorkflow) validateDomain(ctx context.Context, dto CreatePlanDTO, command planCreateCommand) error {
	logger.L(ctx).Infow("CreatePlan validating parameters",
		"action", "create_plan",
		"org_id", dto.OrgID,
		"scale_code", dto.ScaleCode,
		"schedule_type", string(command.scheduleType),
		"trigger_time", command.triggerTime,
		"interval", dto.Interval,
		"total_times", command.totalTimes,
		"fixed_dates_count", len(command.fixedDates),
		"relative_weeks_count", len(dto.RelativeWeeks),
	)
	validator := w.validator
	if validator == nil {
		validator = domainPlan.NewPlanValidator()
	}
	if errs := validator.ValidateForCreation(dto.OrgID, dto.ScaleCode, command.scheduleType, command.triggerTime, dto.Interval, command.totalTimes, command.fixedDates, dto.RelativeWeeks); len(errs) > 0 {
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
		return domainPlan.ToError(errs)
	}
	logger.L(ctx).Infow("CreatePlan validation passed", "action", "create_plan")
	return nil
}

func (w *planCreateWorkflow) createAggregate(ctx context.Context, dto CreatePlanDTO, command planCreateCommand) (*domainPlan.AssessmentPlan, error) {
	logger.L(ctx).Infow("CreatePlan creating domain object",
		"action", "create_plan",
		"org_id", dto.OrgID,
		"scale_code", dto.ScaleCode,
		"schedule_type", string(command.scheduleType),
		"trigger_time", command.triggerTime,
		"interval", dto.Interval,
		"total_times", command.totalTimes,
	)
	planAggregate, err := domainPlan.NewAssessmentPlan(dto.OrgID, dto.ScaleCode, command.scheduleType, dto.Interval, command.totalTimes, command.options...)
	if err != nil {
		logger.L(ctx).Errorw("CreatePlan failed to create domain object",
			"action", "create_plan",
			"org_id", dto.OrgID,
			"scale_code", dto.ScaleCode,
			"schedule_type", string(command.scheduleType),
			"trigger_time", command.triggerTime,
			"interval", dto.Interval,
			"total_times", command.totalTimes,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建计划失败")
	}
	logger.L(ctx).Infow("CreatePlan domain object created",
		"action", "create_plan",
		"plan_id", planAggregate.GetID().String(),
	)
	return planAggregate, nil
}

func (w *planCreateWorkflow) save(ctx context.Context, planAggregate *domainPlan.AssessmentPlan) error {
	logger.L(ctx).Infow("CreatePlan saving to repository",
		"action", "create_plan",
		"plan_id", planAggregate.GetID().String(),
	)
	if err := w.planRepo.Save(ctx, planAggregate); err != nil {
		logger.L(ctx).Errorw("CreatePlan failed to save plan",
			"action", "create_plan",
			"plan_id", planAggregate.GetID().String(),
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}
	logger.L(ctx).Infow("CreatePlan plan saved",
		"action", "create_plan",
		"plan_id", planAggregate.GetID().String(),
	)
	return nil
}

func parseFixedDates(ctx context.Context, rawDates []string) ([]time.Time, error) {
	if len(rawDates) == 0 {
		return nil, nil
	}
	logger.L(ctx).Infow("CreatePlan parsing fixed_dates",
		"action", "create_plan",
		"fixed_dates_count", len(rawDates),
		"fixed_dates", rawDates,
	)
	fixedDates := make([]time.Time, 0, len(rawDates))
	for i, dateStr := range rawDates {
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
	return fixedDates, nil
}

func derivePlanTotalTimes(ctx context.Context, scheduleType domainPlan.PlanScheduleType, initialTotalTimes int, fixedDates []time.Time, relativeWeeks []int) int {
	totalTimes := initialTotalTimes
	logger.L(ctx).Infow("CreatePlan calculating total_times",
		"action", "create_plan",
		"initial_total_times", totalTimes,
		"schedule_type", scheduleType,
	)
	switch scheduleType {
	case domainPlan.PlanScheduleCustom:
		if len(relativeWeeks) > 0 {
			totalTimes = len(relativeWeeks)
			logger.L(ctx).Infow("CreatePlan total_times from relative_weeks",
				"action", "create_plan",
				"total_times", totalTimes,
				"relative_weeks_count", len(relativeWeeks),
			)
		}
	case domainPlan.PlanScheduleFixedDate:
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
	return totalTimes
}

func buildPlanOptions(ctx context.Context, triggerTime string, fixedDates []time.Time, relativeWeeks []int) []domainPlan.PlanOption {
	logger.L(ctx).Infow("CreatePlan building plan options",
		"action", "create_plan",
		"trigger_time", triggerTime,
		"has_fixed_dates", len(fixedDates) > 0,
		"has_relative_weeks", len(relativeWeeks) > 0,
	)
	opts := []domainPlan.PlanOption{domainPlan.WithTriggerTime(triggerTime)}
	if len(fixedDates) > 0 {
		opts = append(opts, domainPlan.WithFixedDates(fixedDates))
		logger.L(ctx).Infow("CreatePlan added fixed_dates option",
			"action", "create_plan",
			"fixed_dates_count", len(fixedDates),
		)
	}
	if len(relativeWeeks) > 0 {
		opts = append(opts, domainPlan.WithRelativeWeeks(relativeWeeks))
		logger.L(ctx).Infow("CreatePlan added relative_weeks option",
			"action", "create_plan",
			"relative_weeks_count", len(relativeWeeks),
		)
	}
	logger.L(ctx).Infow("CreatePlan plan options built",
		"action", "create_plan",
		"options_count", len(opts),
	)
	return opts
}
