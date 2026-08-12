package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	evaluationscheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type LeaseRecoveryRunner struct {
	name      string
	opts      *apiserveroptions.LeaseRecoveryOptions
	recoverer evaluationscheduler.LeaseRecoverer
	leader    leaderLeaseRunner
	now       func() time.Time
}

func NewEvaluationLeaseRecoveryRunner(opts *apiserveroptions.LeaseRecoveryOptions, recoverer evaluationscheduler.LeaseRecoverer, lockManager locklease.Manager, lockBuilder *keyspace.Builder) *LeaseRecoveryRunner {
	return newLeaseRecoveryRunner(opts, recoverer, lockManager, lockBuilder, "evaluation_lease_recovery", locklease.WorkloadEvaluationLeaseRecovery)
}

func NewInterpretationLeaseRecoveryRunner(opts *apiserveroptions.LeaseRecoveryOptions, recoverer evaluationscheduler.LeaseRecoverer, lockManager locklease.Manager, lockBuilder *keyspace.Builder) *LeaseRecoveryRunner {
	return newLeaseRecoveryRunner(opts, recoverer, lockManager, lockBuilder, "interpretation_lease_recovery", locklease.WorkloadInterpretationLeaseRecovery)
}

func newLeaseRecoveryRunner(opts *apiserveroptions.LeaseRecoveryOptions, recoverer evaluationscheduler.LeaseRecoverer, lockManager locklease.Manager, lockBuilder *keyspace.Builder, name string, workload locklease.WorkloadID) *LeaseRecoveryRunner {
	if opts == nil || !opts.Enable || recoverer == nil || opts.Interval <= 0 || opts.BatchLimit <= 0 || opts.LockKey == "" || opts.LockTTL <= 0 {
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded(name, "redis_unavailable")
		log.Warnf("%s not started (HA lock unavailable: redis client unavailable)", name)
		return nil
	}
	return &LeaseRecoveryRunner{
		name: name, opts: opts, recoverer: recoverer, now: time.Now,
		leader: newLeaderLock(
			workloadSpec(workload), opts.LockKey, opts.LockTTL, lockBuilder,
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

func (r *LeaseRecoveryRunner) Name() string {
	if r == nil {
		return ""
	}
	return r.name
}

func (r *LeaseRecoveryRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("%s started (interval=%s, batch_limit=%d, lock_key=%s)", r.name, r.opts.Interval, r.opts.BatchLimit, r.leader.DisplayKey())
	go func() {
		for {
			r.executeTick(ctx)
			if !waitForScheduler(ctx, r.opts.Interval) {
				return
			}
		}
	}()
}

func (r *LeaseRecoveryRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil && ctx.Err() == nil {
		log.Warnf("%s failed: %v", r.name, err)
	}
}

func (r *LeaseRecoveryRunner) runOnce(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire " + r.name + " lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("%s tick skipped (lock_key=%s, reason=lock_not_acquired)", r.name, lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release %s lock (lock_key=%s): %v", r.name, lockKey, err)
		},
	}, func(ctx context.Context) error {
		startedAt := r.now()
		recovered, err := r.recoverer.RecoverExpiredLeases(ctx, r.now(), r.opts.BatchLimit)
		leaseRecoveryDuration.WithLabelValues(r.name).Observe(r.now().Sub(startedAt).Seconds())
		if err != nil {
			leaseRecoveryRunsTotal.WithLabelValues(r.name, "error").Inc()
			return err
		}
		leaseRecoveryRunsTotal.WithLabelValues(r.name, "success").Inc()
		leaseRecoveryRecoveredTotal.WithLabelValues(r.name).Add(float64(recovered))
		return nil
	})
}

var (
	leaseRecoveryRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "lease_recovery", Name: "runs_total",
		Help: "Lease recovery scheduler ticks by workload and result.",
	}, []string{"workload", "result"})
	leaseRecoveryRecoveredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "lease_recovery", Name: "recovered_total",
		Help: "Expired leases successfully handed to their recovery path.",
	}, []string{"workload"})
	leaseRecoveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "lease_recovery", Name: "duration_seconds",
		Help: "Wall time of one lease recovery tick.",
		Buckets: prometheus.DefBuckets,
	}, []string{"workload"})
)
