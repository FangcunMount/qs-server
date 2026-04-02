package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func finalizePlanIfDone(
	ctx context.Context,
	action string,
	planRepo domainPlan.AssessmentPlanRepository,
	planLifecycle *domainPlan.PlanLifecycle,
	eventPublisher event.EventPublisher,
	planID domainPlan.AssessmentPlanID,
) error {
	if planRepo == nil || planLifecycle == nil {
		return nil
	}

	p, err := planRepo.FindByID(ctx, planID)
	if err != nil {
		return err
	}

	finished, err := planLifecycle.TryFinish(ctx, p)
	if err != nil || !finished {
		return err
	}

	if err := planRepo.Save(ctx, p); err != nil {
		return err
	}

	if eventPublisher == nil {
		p.ClearEvents()
		return nil
	}

	for _, evt := range p.Events() {
		if err := eventPublisher.Publish(ctx, evt); err != nil {
			logger.L(ctx).Errorw("Failed to publish plan event",
				"action", action,
				"plan_id", planID.String(),
				"event_type", evt.EventType(),
				"error", err.Error(),
			)
		}
	}
	p.ClearEvents()

	return nil
}
