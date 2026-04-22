package plan

import (
	"context"

	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func saveEntity(
	ctx context.Context,
	isZero bool,
	beforeCreate func() error,
	existsByID func(context.Context) (bool, error),
	create func(context.Context) error,
	update func(context.Context) error,
) error {
	if isZero {
		if err := beforeCreate(); err != nil {
			return err
		}
		return create(ctx)
	}

	exists, err := existsByID(ctx)
	if err != nil {
		return err
	}
	if !exists {
		if err := beforeCreate(); err != nil {
			return err
		}
		return create(ctx)
	}
	return update(ctx)
}

type identifiablePlanEntity interface {
	GetID() meta.ID
}

func saveMappedEntity[PO any, E identifiablePlanEntity](
	ctx context.Context,
	entity E,
	po *PO,
	beforeCreate func() error,
	existsByID func(context.Context, uint64) (bool, error),
	create func(context.Context, *PO, E) error,
	update func(context.Context, *PO, E) error,
) error {
	return saveEntity(
		ctx,
		entity.GetID().IsZero(),
		beforeCreate,
		func(ctx context.Context) (bool, error) { return existsByID(ctx, entity.GetID().Uint64()) },
		func(ctx context.Context) error { return create(ctx, po, entity) },
		func(ctx context.Context) error { return update(ctx, po, entity) },
	)
}

func syncPlanPO(po *AssessmentPlanPO, plan *domainPlan.AssessmentPlan, mapper *PlanMapper) {
	mapper.SyncID(po, plan)
}

func syncTaskPO(po *AssessmentTaskPO, task *domainPlan.AssessmentTask, mapper *TaskMapper) {
	mapper.SyncID(po, task)
}
