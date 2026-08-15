package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type MongoConsistencyAuditRunner struct {
	opts        *apiserveroptions.MongoConsistencyAuditOptions
	service     mongoconsistency.RunnerService
	leader      leaderLeaseRunner
	nextCycleAt time.Time
}

func NewMongoConsistencyAuditRunner(
	opts *apiserveroptions.MongoConsistencyAuditOptions,
	service mongoconsistency.RunnerService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *MongoConsistencyAuditRunner {
	if opts == nil || !opts.Enable {
		mongoconsistency.SetEnabled(false)
		return nil
	}
	mongoconsistency.SetEnabled(true)
	if service == nil {
		log.Warnf("mongo consistency audit not started (service unavailable)")
		return nil
	}
	if opts.TickInterval <= 0 || opts.BatchSize <= 0 || opts.BatchTimeout <= 0 || opts.CycleInterval <= 0 || opts.MaxSamples < 0 || opts.LockKey == "" || opts.LockTTL <= 0 {
		log.Warnf("mongo consistency audit not started (invalid options)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("mongo_consistency_audit", "redis_unavailable")
		log.Warnf("mongo consistency audit not started (HA lock unavailable)")
		return nil
	}
	return &MongoConsistencyAuditRunner{
		opts: opts, service: service,
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadMongoConsistencyAudit), opts.LockKey, opts.LockTTL, lockBuilder,
			func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
				return lockManager.AcquireSpec(ctx, spec, key, ttl)
			},
			func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
				return lockManager.ReleaseSpec(ctx, spec, key, lease)
			},
			leaseRunner(lockManager),
		),
	}
}

func (r *MongoConsistencyAuditRunner) Name() string { return "mongo_consistency_audit" }

func (r *MongoConsistencyAuditRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("mongo consistency audit started (initial_delay=%s tick_interval=%s cycle_interval=%s batch_size=%d batch_timeout=%s max_samples=%d lock_key=%s lock_ttl=%s)",
		r.opts.InitialDelay, r.opts.TickInterval, r.opts.CycleInterval, r.opts.BatchSize, r.opts.BatchTimeout, r.opts.MaxSamples, r.leader.DisplayKey(), r.opts.LockTTL)
	go func() {
		initial := time.NewTimer(r.opts.InitialDelay)
		defer initial.Stop()
		select {
		case <-ctx.Done():
			return
		case <-initial.C:
			r.executeTick(ctx)
		}
		ticker := time.NewTicker(r.opts.TickInterval)
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

func (r *MongoConsistencyAuditRunner) executeTick(ctx context.Context) {
	if !r.nextCycleAt.IsZero() && time.Now().Before(r.nextCycleAt) {
		return
	}
	startedAt := time.Now()
	outcome, err := r.runOnce(ctx)
	duration := time.Since(startedAt)
	if err != nil {
		status := "error"
		if errors.Is(err, context.DeadlineExceeded) {
			status = "timeout"
		}
		log.Warnf("mongo consistency audit batch (cycle=%s phase=%s cursor=%d upper_bound=%d scanned=%d findings=%d duration=%s status=%s error=%v)",
			outcome.CycleID, outcome.Phase, outcome.Cursor, outcome.UpperBound, outcome.Scanned, outcome.Findings, duration, status, err)
		return
	}
	if outcome.Idle {
		r.nextCycleAt = outcome.NextCycleAt
		return
	}
	status := "advanced"
	if outcome.Completed {
		status = "completed"
		r.nextCycleAt = outcome.NextCycleAt
	} else if outcome.Scanned == 0 && outcome.Cursor == 0 {
		status = "initialized"
	}
	// Only bounded internal IDs retained by the checkpoint are sampled. Logs
	// intentionally contain counts and cursor metadata, never business content.
	log.Infof("mongo consistency audit batch (cycle=%s phase=%s cursor=%d upper_bound=%d scanned=%d findings=%d duration=%s status=%s)",
		outcome.CycleID, outcome.Phase, outcome.Cursor, outcome.UpperBound, outcome.Scanned, outcome.Findings, duration, status)
}

func (r *MongoConsistencyAuditRunner) runOnce(ctx context.Context) (mongoconsistency.BatchOutcome, error) {
	var outcome mongoconsistency.BatchOutcome
	err := r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire mongo consistency audit lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("mongo consistency audit tick skipped (lock_key=%s reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release mongo consistency audit lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(leaseCtx context.Context) error {
		batchCtx, cancel := context.WithTimeout(leaseCtx, r.opts.BatchTimeout)
		defer cancel()
		var err error
		outcome, err = r.service.RunAuditBatch(batchCtx, mongoconsistency.RunOptions{
			BatchSize: r.opts.BatchSize, BatchTimeout: r.opts.BatchTimeout,
			CycleInterval: r.opts.CycleInterval, MaxSamples: r.opts.MaxSamples,
		})
		return err
	})
	return outcome, err
}
