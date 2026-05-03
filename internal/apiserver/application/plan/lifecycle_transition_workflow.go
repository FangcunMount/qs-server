package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type planTransitionSpec struct {
	action          string
	startLog        string
	transitionLog   string
	transitionError string
	planSaveError   string
	taskSaveError   string
	successLog      string
}

type planTransitionWorkflow struct {
	planRepo       domainPlan.AssessmentPlanRepository
	taskRepo       domainPlan.AssessmentTaskRepository
	eventPublisher event.EventPublisher
}

func newPlanTransitionWorkflow(
	planRepo domainPlan.AssessmentPlanRepository,
	taskRepo domainPlan.AssessmentTaskRepository,
	eventPublisher event.EventPublisher,
) *planTransitionWorkflow {
	return &planTransitionWorkflow{
		planRepo:       planRepo,
		taskRepo:       taskRepo,
		eventPublisher: eventPublisher,
	}
}

func (w *planTransitionWorkflow) transitionPlanWithTaskCancellation(
	ctx context.Context,
	orgID int64,
	planID string,
	spec planTransitionSpec,
	transition func(context.Context, *domainPlan.AssessmentPlan) ([]*domainPlan.AssessmentTask, error),
) (*PlanResult, error) {
	logger.L(ctx).Infow(spec.startLog,
		"action", spec.action,
		"org_id", orgID,
		"plan_id", planID,
	)

	planAggregate, err := loadPlanInOrg(ctx, w.planRepo, orgID, planID, spec.action)
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

	if err := w.planRepo.Save(ctx, planAggregate); err != nil {
		logger.L(ctx).Errorw(spec.planSaveError,
			"action", spec.action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
	}

	savedTaskCount := w.saveCanceledTasks(ctx, spec.action, planID, spec.taskSaveError, canceledTasks)

	logger.L(ctx).Infow(spec.successLog,
		"action", spec.action,
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(planAggregate), nil
}

func (w *planTransitionWorkflow) saveCanceledTasks(
	ctx context.Context,
	action string,
	planID string,
	taskSaveError string,
	tasks []*domainPlan.AssessmentTask,
) int {
	savedTaskCount := 0
	for _, task := range tasks {
		if err := w.taskRepo.Save(ctx, task); err != nil {
			logger.L(ctx).Errorw(taskSaveError,
				"action", action,
				"plan_id", planID,
				"task_id", task.GetID().String(),
				"error", err.Error(),
			)
			continue
		}
		savedTaskCount++

		eventing.PublishCollectedEvents(ctx, w.eventPublisher, task, nil, func(evt event.DomainEvent, err error) {
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
