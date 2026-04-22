package plan

import (
	"context"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type scopedResourceLoader[T any, ID any] struct {
	resourceKey   string
	resourceType  string
	parse         func(string) (ID, error)
	find          func(context.Context, ID) (T, error)
	orgID         func(T) int64
	invalidError  func(error) error
	notFoundError func() error
	scopeError    func() error
}

func invalidArgumentErr(format string, args ...interface{}) error {
	return pkgerrors.WithCode(errorCode.ErrInvalidArgument, format, args...)
}

func wrapDatabaseErr(err error, msg string) error {
	return pkgerrors.WrapC(err, errorCode.ErrDatabase, "%s", msg)
}

func loadScopedResource[T any, ID any](
	ctx context.Context,
	loader scopedResourceLoader[T, ID],
	orgID int64,
	rawID string,
	action string,
) (T, error) {
	var zero T

	id, err := loader.parse(rawID)
	if err != nil {
		logger.L(ctx).Errorw("Invalid "+loader.resourceType+" ID",
			"action", action,
			loader.resourceKey, rawID,
			"error", err.Error(),
		)
		return zero, loader.invalidError(err)
	}

	resource, err := loader.find(ctx, id)
	if err != nil {
		logger.L(ctx).Errorw(loader.resourceType+" not found",
			"action", action,
			loader.resourceKey, rawID,
			"error", err.Error(),
		)
		return zero, loader.notFoundError()
	}

	resourceOrgID := loader.orgID(resource)
	if resourceOrgID != orgID {
		logger.L(ctx).Warnw(loader.resourceType+" access denied due to org scope mismatch",
			"action", action,
			loader.resourceKey, rawID,
			"request_org_id", orgID,
			"resource_org_id", resourceOrgID,
		)
		return zero, loader.scopeError()
	}

	return resource, nil
}

func loadPlanInOrg(
	ctx context.Context,
	repo domainplan.AssessmentPlanRepository,
	orgID int64,
	planID string,
	action string,
) (*domainplan.AssessmentPlan, error) {
	return loadScopedResource(ctx, scopedResourceLoader[*domainplan.AssessmentPlan, domainplan.AssessmentPlanID]{
		resourceKey:  "plan_id",
		resourceType: "plan",
		parse:        toPlanID,
		find:         repo.FindByID,
		orgID: func(planAggregate *domainplan.AssessmentPlan) int64 {
			return planAggregate.GetOrgID()
		},
		invalidError: func(err error) error {
			return invalidArgumentErr("无效的计划ID: %v", err)
		},
		notFoundError: func() error {
			return pkgerrors.WithCode(errorCode.ErrPageNotFound, "计划不存在")
		},
		scopeError: func() error {
			return pkgerrors.WithCode(errorCode.ErrPermissionDenied, "计划不属于当前机构")
		},
	}, orgID, planID, action)
}

func loadTaskInOrg(
	ctx context.Context,
	repo domainplan.AssessmentTaskRepository,
	orgID int64,
	taskID string,
	action string,
) (*domainplan.AssessmentTask, error) {
	return loadScopedResource(ctx, scopedResourceLoader[*domainplan.AssessmentTask, domainplan.AssessmentTaskID]{
		resourceKey:  "task_id",
		resourceType: "task",
		parse:        toTaskID,
		find:         repo.FindByID,
		orgID: func(task *domainplan.AssessmentTask) int64 {
			return task.GetOrgID()
		},
		invalidError: func(err error) error {
			return invalidArgumentErr("无效的任务ID: %v", err)
		},
		notFoundError: func() error {
			return pkgerrors.WithCode(errorCode.ErrPageNotFound, "任务不存在")
		},
		scopeError: func() error {
			return pkgerrors.WithCode(errorCode.ErrPermissionDenied, "任务不属于当前机构")
		},
	}, orgID, taskID, action)
}
