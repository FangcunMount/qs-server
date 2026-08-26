package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

const (
	roleProjectionReconcileInterval = 10 * time.Minute
	roleProjectionReconcileBatch    = 100
	roleProjectionReconcileLeaseKey = "global"
)

// AuthzRoleProjectionRunner periodically converges the non-authoritative staff
// role projection after IAM assignment changes.
type AuthzRoleProjectionRunner struct {
	reconciler operatorapp.OperatorRoleProjectionReconciler
	locks      locklease.Runner
	interval   time.Duration
	batchSize  int
}

func NewAuthzRoleProjectionRunner(reconciler operatorapp.OperatorRoleProjectionReconciler, locks locklease.Runner) *AuthzRoleProjectionRunner {
	if reconciler == nil || locks == nil {
		return nil
	}
	return &AuthzRoleProjectionRunner{
		reconciler: reconciler,
		locks:      locks,
		interval:   roleProjectionReconcileInterval,
		batchSize:  roleProjectionReconcileBatch,
	}
}

func (r *AuthzRoleProjectionRunner) Name() string { return "authz_role_projection_reconcile" }

func (r *AuthzRoleProjectionRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("authz role projection reconciler started (interval=%s, batch_size=%d)", r.interval, r.batchSize)
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.runOnce(ctx); err != nil && ctx.Err() == nil {
					log.Warnf("authz role projection reconcile failed: %v", err)
				}
			}
		}
	}()
}

func (r *AuthzRoleProjectionRunner) runOnce(ctx context.Context) error {
	_, err := r.locks.Run(ctx, locklease.WorkloadAuthzRoleProjectionReconcile, roleProjectionReconcileLeaseKey, 0, func(leaseCtx context.Context) error {
		totalScanned := 0
		totalSucceeded := 0
		for {
			result, reconcileErr := r.reconciler.ReconcilePending(leaseCtx, r.batchSize)
			totalScanned += result.Scanned
			totalSucceeded += result.Succeeded
			if reconcileErr != nil {
				return reconcileErr
			}
			if result.Scanned < r.batchSize {
				break
			}
		}
		if totalScanned > 0 {
			log.Infof("authz role projection reconcile completed (scanned=%d, succeeded=%d)", totalScanned, totalSucceeded)
		}
		return nil
	})
	return err
}
