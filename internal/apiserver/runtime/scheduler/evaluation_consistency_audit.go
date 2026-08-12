package scheduler

import (
	"context"
	"fmt"
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

type EvaluationConsistencyAuditRunner struct {
	opts    *apiserveroptions.EvaluationConsistencyAuditOptions
	service evaluationscheduler.Service
	leader  leaderLeaseRunner
	now     func() time.Time
}

func NewEvaluationConsistencyAuditRunner(
	opts *apiserveroptions.EvaluationConsistencyAuditOptions,
	service evaluationscheduler.Service,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *EvaluationConsistencyAuditRunner {
	return newEvaluationConsistencyAuditRunnerWithHooks(
		opts, service, lockManager, lockBuilder,
		func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newEvaluationConsistencyAuditRunnerWithHooks(
	opts *apiserveroptions.EvaluationConsistencyAuditOptions,
	service evaluationscheduler.Service,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock leaderLockAcquireFunc,
	releaseLock leaderLockReleaseFunc,
) *EvaluationConsistencyAuditRunner {
	if opts == nil || !opts.Enable || service == nil || opts.InitialDelay < 0 || opts.BatchInterval <= 0 ||
		opts.CycleInterval <= 0 || opts.BatchSize <= 0 || opts.BatchTimeout <= 0 || opts.LockKey == "" || opts.LockTTL <= 0 {
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("evaluation_consistency_audit", "redis_unavailable")
		log.Warnf("evaluation consistency audit not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		return nil
	}
	return &EvaluationConsistencyAuditRunner{
		opts: opts, service: service, now: time.Now,
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadEvaluationConsistencyAudit), opts.LockKey, opts.LockTTL,
			lockBuilder, acquireLock, releaseLock, leaseRunner(lockManager),
		),
	}
}

func (r *EvaluationConsistencyAuditRunner) Name() string { return "evaluation_consistency_audit" }

func (r *EvaluationConsistencyAuditRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("evaluation consistency audit started (initial_delay=%s, batch_interval=%s, cycle_interval=%s, batch_size=%d, lock_key=%s)",
		r.opts.InitialDelay, r.opts.BatchInterval, r.opts.CycleInterval, r.opts.BatchSize, r.leader.DisplayKey())
	go func() {
		if !waitForScheduler(ctx, r.opts.InitialDelay) {
			return
		}
		for {
			cycleStartedAt := time.Now()
			if err := r.runCycle(ctx); err != nil && ctx.Err() == nil {
				log.Warnf("evaluation consistency audit cycle failed: %v", err)
			}
			waitDuration := r.opts.CycleInterval - time.Since(cycleStartedAt)
			if !waitForScheduler(ctx, waitDuration) {
				return
			}
		}
	}()
}

func (r *EvaluationConsistencyAuditRunner) runCycle(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire evaluation consistency audit lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("evaluation consistency audit cycle skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release evaluation consistency audit lock (lock_key=%s): %v", lockKey, err)
		},
	}, r.executeCycle)
}

func (r *EvaluationConsistencyAuditRunner) executeCycle(ctx context.Context) (cycleErr error) {
	startedAt := r.now()
	resultLabel := "success"
	defer func() {
		if cycleErr != nil {
			resultLabel = "error"
		}
		evaluationConsistencyAuditCyclesTotal.WithLabelValues(resultLabel).Inc()
		evaluationConsistencyAuditCycleDuration.Observe(r.now().Sub(startedAt).Seconds())
	}()

	var cursor uint64
	totalScanned := 0
	for {
		batchCtx, cancel := context.WithTimeout(ctx, r.opts.BatchTimeout)
		result, err := r.service.AuditBatch(batchCtx, cursor, r.opts.BatchSize)
		cancel()
		if err != nil {
			evaluationConsistencyAuditBatchesTotal.WithLabelValues("error").Inc()
			return err
		}
		evaluationConsistencyAuditBatchesTotal.WithLabelValues("success").Inc()
		evaluationConsistencyAuditCandidatesTotal.Add(float64(result.Scanned))
		totalScanned += result.Scanned
		if result.NextCursor > 0 {
			evaluationConsistencyAuditWatermark.Set(float64(result.NextCursor))
		}
		if result.CycleComplete {
			completedWatermark := cursor
			if result.NextCursor > completedWatermark {
				completedWatermark = result.NextCursor
			}
			evaluationConsistencyAuditLastCycleScanned.Set(float64(totalScanned))
			evaluationConsistencyAuditLastSuccess.Set(float64(r.now().Unix()))
			log.Infof("evaluation consistency audit cycle completed (scanned=%d, watermark_assessment_id=%d)", totalScanned, completedWatermark)
			return nil
		}
		if result.Scanned <= 0 || result.NextCursor <= cursor {
			return fmt.Errorf("evaluation consistency audit made no watermark progress (cursor=%d, next=%d, scanned=%d)", cursor, result.NextCursor, result.Scanned)
		}
		cursor = result.NextCursor
		if !waitForScheduler(ctx, r.opts.BatchInterval) {
			return ctx.Err()
		}
	}
}

func waitForScheduler(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var (
	evaluationConsistencyAuditBatchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "batches_total",
		Help: "Evaluation consistency audit batches by result.",
	}, []string{"result"})
	evaluationConsistencyAuditCandidatesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "candidates_scanned_total",
		Help: "Assessment evidence rows scanned by the Evaluation consistency audit.",
	})
	evaluationConsistencyAuditCyclesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "cycles_total",
		Help: "Complete Evaluation consistency audit cycles by result.",
	}, []string{"result"})
	evaluationConsistencyAuditCycleDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "cycle_duration_seconds",
		Help:    "Wall time of one complete Evaluation consistency audit cycle.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	})
	evaluationConsistencyAuditWatermark = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "watermark_assessment_id",
		Help: "Latest Assessment ID reached by the active or last audit cycle.",
	})
	evaluationConsistencyAuditLastCycleScanned = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "last_cycle_scanned",
		Help: "Assessment evidence rows scanned in the last successful full cycle.",
	})
	evaluationConsistencyAuditLastSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "qs", Subsystem: "evaluation_consistency_audit", Name: "last_success_unixtime",
		Help: "Unix timestamp of the last successful full Evaluation consistency audit cycle.",
	})
)
