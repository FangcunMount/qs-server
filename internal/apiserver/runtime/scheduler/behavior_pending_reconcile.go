package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

// BehaviorPendingReconcileRunner periodically retries pending behavior attribution work.
type BehaviorPendingReconcileRunner struct {
	opts      *apiserveroptions.BehaviorPendingReconcileOptions
	projector statisticsApp.BehaviorProjectorService
	leader    leaderLeaseRunner
}

// NewBehaviorPendingReconcileRunner creates the reconcile runner when dependencies are available.
func NewBehaviorPendingReconcileRunner(
	opts *apiserveroptions.BehaviorPendingReconcileOptions,
	projector statisticsApp.BehaviorProjectorService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *BehaviorPendingReconcileRunner {
	return newBehaviorPendingReconcileRunnerWithHooks(
		opts,
		projector,
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

func newBehaviorPendingReconcileRunnerWithHooks(
	opts *apiserveroptions.BehaviorPendingReconcileOptions,
	projector statisticsApp.BehaviorProjectorService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error,
) *BehaviorPendingReconcileRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if projector == nil {
		log.Warnf("behavior pending reconcile not started (projector unavailable)")
		return nil
	}
	if opts.Interval <= 0 {
		log.Warnf("behavior pending reconcile not started (interval must be greater than 0)")
		return nil
	}
	if opts.BatchLimit <= 0 {
		log.Warnf("behavior pending reconcile not started (batch_limit must be greater than 0)")
		return nil
	}
	if opts.LockKey == "" {
		log.Warnf("behavior pending reconcile not started (lock_key is empty)")
		return nil
	}
	if opts.LockTTL <= 0 {
		log.Warnf("behavior pending reconcile not started (lock_ttl must be greater than 0)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("behavior_pending_reconcile", "redis_unavailable")
		log.Warnf("behavior pending reconcile not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("behavior pending reconcile not started (lock hooks unavailable)")
		return nil
	}

	return &BehaviorPendingReconcileRunner{
		opts:      opts,
		projector: projector,
		leader: newLeaderLock(
			locklease.Specs.BehaviorPendingReconcile,
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
		),
	}
}

// Name returns the runner name.
func (r *BehaviorPendingReconcileRunner) Name() string {
	return "behavior_pending_reconcile"
}

// Start starts the reconcile ticker loop.
func (r *BehaviorPendingReconcileRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}

	lockKey := r.lockKey()
	log.Infof("behavior pending reconcile started (interval=%s, batch_limit=%d, lock_key=%s, lock_ttl=%s)",
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

func (r *BehaviorPendingReconcileRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("behavior pending reconcile failed: %v", err)
	}
}

func (r *BehaviorPendingReconcileRunner) runOnce(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire behavior pending reconcile lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("behavior pending reconcile tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release behavior pending reconcile lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		_, err := r.projector.ReconcilePendingBehaviorEvents(ctx, r.opts.BatchLimit)
		return err
	})
}

func (r *BehaviorPendingReconcileRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
