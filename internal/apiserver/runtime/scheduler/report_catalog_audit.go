package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type ReportCatalogAuditRunner struct {
	opts        *apiserveroptions.ReportCatalogAuditOptions
	service     interpretationcatalog.RunnerService
	leader      leaderLeaseRunner
	nextCycleAt time.Time
}

func NewReportCatalogAuditRunner(
	opts *apiserveroptions.ReportCatalogAuditOptions,
	service interpretationcatalog.RunnerService,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *ReportCatalogAuditRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if service == nil {
		log.Warnf("report catalog audit not started (service unavailable)")
		return nil
	}
	if opts.TickInterval <= 0 || opts.BatchSize <= 0 || opts.BatchTimeout <= 0 || opts.CycleInterval <= 0 || opts.LockKey == "" || opts.LockTTL <= 0 {
		log.Warnf("report catalog audit not started (invalid options)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("report_catalog_audit", "redis_unavailable")
		log.Warnf("report catalog audit not started (HA lock unavailable)")
		return nil
	}
	return &ReportCatalogAuditRunner{
		opts: opts, service: service,
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadReportCatalogAudit), opts.LockKey, opts.LockTTL, lockBuilder,
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

func (r *ReportCatalogAuditRunner) Name() string { return "report_catalog_audit" }

func (r *ReportCatalogAuditRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("report catalog audit started (initial_delay=%s tick_interval=%s cycle_interval=%s batch_size=%d batch_timeout=%s lock_key=%s lock_ttl=%s)",
		r.opts.InitialDelay, r.opts.TickInterval, r.opts.CycleInterval, r.opts.BatchSize, r.opts.BatchTimeout, r.leader.DisplayKey(), r.opts.LockTTL)
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

func (r *ReportCatalogAuditRunner) executeTick(ctx context.Context) {
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
		log.Warnf("report catalog audit batch (cycle=%s phase=%s cursor=%d upper_bound=%d scanned=%d findings=%d duration=%s status=%s error=%v)",
			outcome.CycleID, outcome.Phase, outcome.Cursor, outcome.UpperBound, outcome.Scanned, outcome.Findings, duration, status, err)
		return
	}
	if outcome.Idle {
		r.nextCycleAt = outcome.NextCycleAt
		log.Debugf("report catalog audit tick skipped (cycle=%s status=cycle_not_due)", outcome.CycleID)
		return
	}
	status := "advanced"
	if outcome.Completed {
		status = "completed"
		r.nextCycleAt = outcome.NextCycleAt
	} else if outcome.Scanned == 0 && outcome.Cursor == 0 {
		status = "initialized"
	}
	log.Infof("report catalog audit batch (cycle=%s phase=%s cursor=%d upper_bound=%d scanned=%d findings=%d duration=%s status=%s)",
		outcome.CycleID, outcome.Phase, outcome.Cursor, outcome.UpperBound, outcome.Scanned, outcome.Findings, duration, status)
}

func (r *ReportCatalogAuditRunner) runOnce(ctx context.Context) (interpretationcatalog.AuditBatchOutcome, error) {
	var outcome interpretationcatalog.AuditBatchOutcome
	err := r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire report catalog audit lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("report catalog audit tick skipped (lock_key=%s reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release report catalog audit lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(leaseCtx context.Context) error {
		batchCtx, cancel := context.WithTimeout(leaseCtx, r.opts.BatchTimeout)
		defer cancel()
		var err error
		outcome, err = r.service.RunAuditBatch(batchCtx, interpretationcatalog.AuditRunOptions{
			BatchSize: r.opts.BatchSize, BatchTimeout: r.opts.BatchTimeout, CycleInterval: r.opts.CycleInterval,
		})
		return err
	})
	return outcome, err
}
