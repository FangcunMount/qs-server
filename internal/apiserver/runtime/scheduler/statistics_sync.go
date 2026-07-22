package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type statisticsCoordinator interface {
	Run(context.Context, statisticsApp.RunRequest) (*statisticsApp.Run, error)
}

type StatisticsSyncOrgResult struct {
	OrgID  int64
	Status string
}

type StatisticsSyncSummary struct {
	Orgs      []StatisticsSyncOrgResult
	Succeeded int
	Failed    int
}

type StatisticsSyncPartialError struct{ Summary StatisticsSyncSummary }

func (e *StatisticsSyncPartialError) Error() string {
	return fmt.Sprintf("statistics sync partially failed: succeeded=%d failed=%d", e.Summary.Succeeded, e.Summary.Failed)
}

type StatisticsSyncRunner struct {
	opts        *apiserveroptions.StatisticsSyncOptions
	coordinator statisticsCoordinator
	leader      leaderLeaseRunner
	clock       DailyClock
	now         func() time.Time
}

func NewStatisticsSyncRunner(
	opts *apiserveroptions.StatisticsSyncOptions,
	coordinator statisticsCoordinator,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
) *StatisticsSyncRunner {
	return newStatisticsSyncRunnerWithHooks(
		opts,
		coordinator,
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

func newStatisticsSyncRunnerWithHooks(
	opts *apiserveroptions.StatisticsSyncOptions,
	coordinator statisticsCoordinator,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock func(context.Context, locklease.Spec, string, time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(context.Context, locklease.Spec, string, *locklease.Lease) error,
) *StatisticsSyncRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if coordinator == nil {
		log.Warnf("statistics sync scheduler not started (coordinator unavailable)")
		return nil
	}
	if len(opts.OrgIDs) == 0 {
		log.Warnf("statistics sync scheduler not started (org_ids is empty)")
		return nil
	}
	clock, err := ParseDailyClock(opts.RunAt)
	if err != nil {
		log.Warnf("statistics sync scheduler disabled: invalid run_at %q: %v", opts.RunAt, err)
		return nil
	}
	if opts.RepairWindowDays <= 0 || opts.LockKey == "" || opts.LockTTL <= 0 {
		log.Warnf("statistics sync scheduler not started (invalid repair window or lock settings)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("statistics_sync_leader", "redis_unavailable")
		log.Warnf("statistics sync scheduler not started (HA lock unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		return nil
	}
	return &StatisticsSyncRunner{
		opts:        opts,
		coordinator: coordinator,
		leader: newLeaderLock(
			workloadSpec(locklease.WorkloadStatisticsSyncLeader),
			opts.LockKey,
			opts.LockTTL,
			lockBuilder,
			acquireLock,
			releaseLock,
			leaseRunner(lockManager),
		),
		clock: clock,
		now:   time.Now,
	}
}

func (r *StatisticsSyncRunner) Name() string { return "statistics_sync" }

func (r *StatisticsSyncRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	log.Infof("statistics sync scheduler started (org_ids=%v, run_at=%s, repair_window_days=%d, lock_key=%s, lock_ttl=%s)", r.opts.OrgIDs, r.opts.RunAt, r.opts.RepairWindowDays, r.lockKey(), r.opts.LockTTL)
	go func() {
		for {
			now := r.now().In(statisticsDomain.Shanghai)
			timer := time.NewTimer(time.Until(NextDailyRun(now, r.clock.Hour, r.clock.Minute)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if err := r.runOnce(ctx); err != nil {
				log.Warnf("statistics sync scheduler tick failed: %v", err)
			}
		}
	}()
}

func (r *StatisticsSyncRunner) runOnce(ctx context.Context) error {
	return r.leader.Run(ctx, leaderLockRunOptions{
		AcquireError: "failed to acquire statistics sync scheduler lock",
		OnNotAcquired: func(lockKey string) {
			log.Debugf("statistics sync scheduler tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		},
		OnReleaseError: func(lockKey string, err error) {
			log.Warnf("failed to release statistics sync scheduler lock (lock_key=%s): %v", lockKey, err)
		},
	}, func(ctx context.Context) error {
		summary := StatisticsSyncSummary{Orgs: make([]StatisticsSyncOrgResult, 0, len(r.opts.OrgIDs))}
		for _, orgID := range r.opts.OrgIDs {
			start, end := statisticsSyncRepairWindow(r.now().In(statisticsDomain.Shanghai), r.opts.RepairWindowDays)
			toDate := end.AddDate(0, 0, -1)
			run, err := r.coordinator.Run(ctx, statisticsApp.RunRequest{OrgID: orgID, FromDate: start, ToDate: toDate, Reason: "nightly statistics publish", TriggerType: "scheduled", Mode: statisticsDomain.RunModePublish})
			statusValue := "failed"
			if run != nil {
				statusValue = string(run.Status)
			}
			if err != nil {
				summary.Failed++
				log.Warnf("statistics nightly run failed (org=%d,status=%s): %v", orgID, statusValue, err)
			} else {
				summary.Succeeded++
			}
			observeStatisticsSchedulerOrg(statusValue)
			summary.Orgs = append(summary.Orgs, StatisticsSyncOrgResult{OrgID: orgID, Status: statusValue})
		}
		if summary.Failed > 0 {
			return &StatisticsSyncPartialError{Summary: summary}
		}
		return nil
	})
}

func (r *StatisticsSyncRunner) lockKey() string {
	if r == nil {
		return ""
	}
	return r.leader.DisplayKey()
}

func statisticsSyncRepairWindow(now time.Time, repairWindowDays int) (time.Time, time.Time) {
	if repairWindowDays <= 0 {
		repairWindowDays = 7
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -repairWindowDays), todayStart
}
