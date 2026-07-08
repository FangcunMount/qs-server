package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	consistencyApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

// EvaluationConsistencyReconcileRunner periodically repairs scoring/reporting cross-store drift.
type EvaluationConsistencyReconcileRunner struct {
	opts    *apiserveroptions.EvaluationConsistencyReconcileOptions
	service consistencyApp.Service
	leader  leaderLeaseRunner
}

// NewEvaluationConsistencyReconcileRunner creates the reconcile runner when dependencies are available.
func NewEvaluationConsistencyReconcileRunner(
	opts *apiserveroptions.EvaluationConsistencyReconcileOptions,
	service consistencyApp.Service,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *EvaluationConsistencyReconcileRunner {
	return newEvaluationConsistencyReconcileRunnerWithHooks(
		opts,
		service,
		lockManager,
		lockBuilder,
		func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newEvaluationConsistencyReconcileRunnerWithHooks(
	opts *apiserveroptions.EvaluationConsistencyReconcileOptions,
	service consistencyApp.Service,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error,
) *EvaluationConsistencyReconcileRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if service == nil {
		log.Warnf("evaluation consistency reconcile not started (service unavailable)")
		return nil
	}
	if opts.Interval <= 0 {
		log.Warnf("evaluation consistency reconcile not started (interval must be greater than 0)")
		return nil
	}
	if opts.BatchLimit <= 0 {
		log.Warnf("evaluation consistency reconcile not started (batch_limit must be greater than 0)")
		return nil
	}
	if opts.LockKey == "" {
		log.Warnf("evaluation consistency reconcile not started (lock_key is empty)")
		return nil
	}
	if opts.LockTTL <= 0 {
		log.Warnf("evaluation consistency reconcile not started (lock_ttl must be greater than 0)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("evaluation_consistency_reconcile", "redis_unavailable")
		log.Warnf("evaluation consistency reconcile not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("evaluation consistency reconcile not started (lock hooks unavailable)")
		return nil
	}

	return &EvaluationConsistencyReconcileRunner{
		opts:    opts,
		service: service,
		leader: newLeaderLock(
			locklease.Specs.EvaluationConsistencyReconcile,
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
		),
	}
}

// Name returns the runner name.
func (r *EvaluationConsistencyReconcileRunner) Name() string {
	return "evaluation_consistency_reconcile"
}

// Start starts the reconcile ticker loop.
func (r *EvaluationConsistencyReconcileRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}

	lockKey := r.lockKey()
	log.Infof("evaluation consistency reconcile started (interval=%s, batch_limit=%d, lock_key=%s, lock_ttl=%s)",
		r.opts.Interval, r.opts.BatchLimit, lockKey, r.opts.LockTTL)

	go func() {
		r.executeTick(ctx)

		ticker := time.NewTicker(r.opts.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.executeTick(ctx)
			}
		}
	}()
}

func (r *EvaluationConsistencyReconcileRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("evaluation consistency reconcile failed: %v", err)
	}
}

func (r *EvaluationConsistencyReconcileRunner) runOnce(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire evaluation consistency reconcile lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("evaluation consistency reconcile tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release evaluation consistency reconcile lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		_, err := r.service.ReconcileOnce(ctx, r.opts.BatchLimit)
		return err
	})
}

func (r *EvaluationConsistencyReconcileRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
