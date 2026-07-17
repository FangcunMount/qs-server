package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// BehaviorJourneyScanRunner periodically projects behavior journey statistics from fact tables.
type BehaviorJourneyScanRunner struct {
	opts    *apiserveroptions.BehaviorJourneyScanOptions
	scanner statisticsApp.BehaviorJourneyScanService
	leader  leaderLeaseRunner
}

// NewBehaviorJourneyScanRunner creates the scan runner when dependencies are available.
func NewBehaviorJourneyScanRunner(
	opts *apiserveroptions.BehaviorJourneyScanOptions,
	scanner statisticsApp.BehaviorJourneyScanService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *BehaviorJourneyScanRunner {
	return newBehaviorJourneyScanRunnerWithHooks(
		opts,
		scanner,
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

func newBehaviorJourneyScanRunnerWithHooks(
	opts *apiserveroptions.BehaviorJourneyScanOptions,
	scanner statisticsApp.BehaviorJourneyScanService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error,
) *BehaviorJourneyScanRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if scanner == nil {
		log.Warnf("behavior journey scan not started (scanner unavailable)")
		return nil
	}
	if opts.Interval <= 0 {
		log.Warnf("behavior journey scan not started (interval must be greater than 0)")
		return nil
	}
	if opts.BatchSize <= 0 {
		log.Warnf("behavior journey scan not started (batch_size must be greater than 0)")
		return nil
	}
	if opts.LockKey == "" {
		log.Warnf("behavior journey scan not started (lock_key is empty)")
		return nil
	}
	if opts.LockTTL <= 0 {
		log.Warnf("behavior journey scan not started (lock_ttl must be greater than 0)")
		return nil
	}
	if len(opts.OrgIDs) == 0 {
		log.Warnf("behavior journey scan not started (org_ids is empty)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("behavior_journey_scan", "redis_unavailable")
		log.Warnf("behavior journey scan not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("behavior journey scan not started (lock hooks unavailable)")
		return nil
	}
	return &BehaviorJourneyScanRunner{
		opts:    opts,
		scanner: scanner,
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadBehaviorJourneyScanLeader),
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
			leaseRunner(lockManager),
		),
	}
}

// Name returns the runner name.
func (r *BehaviorJourneyScanRunner) Name() string {
	return "behavior_journey_scan"
}

// Start starts the scan loop.
func (r *BehaviorJourneyScanRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("behavior journey scan started (interval=%s, batch_size=%d, lookback=%s, dry_run=%t, lock_key=%s, lock_ttl=%s)",
		r.opts.Interval, r.opts.BatchSize, r.opts.Lookback, r.opts.DryRun, r.lockKey(), r.opts.LockTTL)

	go func() {
		if r.opts.InitialDelay > 0 {
			if !WaitDelay(ctx, r.opts.InitialDelay) {
				return
			}
		}
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

func (r *BehaviorJourneyScanRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("behavior journey scan failed: %v", err)
	}
}

func (r *BehaviorJourneyScanRunner) runOnce(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire behavior journey scan lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("behavior journey scan tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release behavior journey scan lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		result, err := r.scanner.ScanDue(ctx, statisticsApp.BehaviorJourneyScanInput{
			OrgIDs:       r.opts.OrgIDs,
			Sources:      r.opts.Sources,
			BatchSize:    r.opts.BatchSize,
			Lookback:     r.opts.Lookback,
			Now:          time.Now(),
			DryRun:       r.opts.DryRun,
			WindowRecalc: r.opts.WindowRecalc,
		})
		if err == nil {
			log.Infof("behavior journey scan completed (sources=%d, recalculations=%d)", len(result.SourceResults), len(result.RecalcResults))
		}
		return err
	})
}

func (r *BehaviorJourneyScanRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}
