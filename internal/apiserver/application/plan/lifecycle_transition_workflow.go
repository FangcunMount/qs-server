package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type planTransitionSpec struct {
	action           string
	startLog         string
	transitionLog    string
	transitionError  string
	planSaveError    string
	taskSaveError    string
	successLog       string
	enrollmentAction string
}

type planTransitionWorkflow struct {
	planRepo       domainPlan.AssessmentPlanRepository
	taskRepo       domainPlan.AssessmentTaskRepository
	eventPublisher event.EventPublisher
	enrollments    domainPlan.PlanEnrollmentLifecycleRepository
	tx             apptransaction.Runner
}

func newPlanTransitionWorkflow(
	planRepo domainPlan.AssessmentPlanRepository,
	taskRepo domainPlan.AssessmentTaskRepository,
	enrollments domainPlan.PlanEnrollmentLifecycleRepository,
	tx apptransaction.Runner,
	eventPublisher event.EventPublisher,
) *planTransitionWorkflow {
	return &planTransitionWorkflow{
		planRepo:       planRepo,
		taskRepo:       taskRepo,
		eventPublisher: eventPublisher,
		enrollments:    enrollments,
		tx:             tx,
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

	save := func(txCtx context.Context) error {
		if err := w.planRepo.Save(txCtx, planAggregate); err != nil {
			return errors.WrapC(err, errorCode.ErrDatabase, "保存计划失败")
		}
		for _, task := range canceledTasks {
			if err := w.taskRepo.Save(txCtx, task); err != nil {
				return errors.WrapC(err, errorCode.ErrDatabase, "保存取消任务失败")
			}
		}
		if w.enrollments != nil {
			switch spec.enrollmentAction {
			case "terminate":
				_, err = w.enrollments.TerminateActiveByPlan(txCtx, orgID, planAggregate.GetID(), spec.action, time.Now())
			case "close":
				_, err = w.enrollments.CloseActiveByPlanIfAllTasksTerminal(txCtx, orgID, planAggregate.GetID(), time.Now())
			}
			if err != nil {
				return fmt.Errorf("transition plan enrollments: %w", err)
			}
		}
		return nil
	}
	if w.tx != nil {
		err = w.tx.WithinTransaction(ctx, save)
	} else {
		err = save(ctx)
	}
	if err != nil {
		logger.L(ctx).Errorw(spec.planSaveError,
			"action", spec.action,
			"plan_id", planID,
			"error", err.Error(),
		)
		return nil, err
	}

	savedTaskCount := w.publishCanceledTaskEvents(ctx, spec.action, planID, canceledTasks)

	logger.L(ctx).Infow(spec.successLog,
		"action", spec.action,
		"plan_id", planID,
		"canceled_tasks_count", len(canceledTasks),
		"saved_tasks_count", savedTaskCount,
	)

	return toPlanResult(planAggregate), nil
}

func (w *planTransitionWorkflow) publishCanceledTaskEvents(
	ctx context.Context,
	action string,
	planID string,
	tasks []*domainPlan.AssessmentTask,
) int {
	savedTaskCount := 0
	for _, task := range tasks {
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
