package operator

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

// RoleProjectionReconcileResult describes one bounded reconciliation pass.
type RoleProjectionReconcileResult struct {
	Scanned   int
	Succeeded int
	Failed    int
}

// OperatorRoleProjectionReconciler converges pending local read projections.
// It never participates in authorization decisions; IAM remains authoritative.
type OperatorRoleProjectionReconciler interface {
	ReconcilePending(ctx context.Context, limit int) (RoleProjectionReconcileResult, error)
}

type roleProjectionReconciler struct {
	repo  domain.PendingProjectionRepository
	store domain.Repository
	authz iambridge.OperatorAuthzGateway
}

func NewRoleProjectionReconciler(repo domain.Repository, authz iambridge.OperatorAuthzGateway) OperatorRoleProjectionReconciler {
	pending, _ := repo.(domain.PendingProjectionRepository)
	if pending == nil || authz == nil || !authz.IsEnabled() {
		return nil
	}
	return &roleProjectionReconciler{repo: pending, store: repo, authz: authz}
}

func (r *roleProjectionReconciler) ReconcilePending(ctx context.Context, limit int) (RoleProjectionReconcileResult, error) {
	result := RoleProjectionReconcileResult{}
	if r == nil || r.repo == nil || r.store == nil || r.authz == nil || !r.authz.IsEnabled() {
		return result, nil
	}
	operators, err := r.repo.ListAuthzProjectionPending(ctx, limit)
	if err != nil {
		return result, fmt.Errorf("list pending operator role projections: %w", err)
	}
	result.Scanned = len(operators)
	for _, op := range operators {
		if op == nil {
			continue
		}
		projection, loadErr := r.authz.LoadOperatorRoleProjection(ctx, op.OrgID(), op.UserID())
		if loadErr == nil {
			loadErr = persistOperatorRoleProjection(ctx, r.store, op, projection, false)
		}
		if loadErr != nil {
			result.Failed++
			continue
		}
		result.Succeeded++
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("reconcile operator role projections: succeeded=%d failed=%d", result.Succeeded, result.Failed)
	}
	return result, nil
}
