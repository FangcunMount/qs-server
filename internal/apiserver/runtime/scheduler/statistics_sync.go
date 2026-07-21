package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsV2App "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	statisticsV2Domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type statisticsSyncService interface {
	SyncDailyStatistics(ctx context.Context, orgID int64, opts statisticsApp.SyncDailyOptions) error
	SyncOrgSnapshotStatistics(ctx context.Context, orgID int64) error
	SyncPlanStatistics(ctx context.Context, orgID int64) error
}

type statisticsV2Coordinator interface {
	Run(context.Context, statisticsV2App.RunRequest) (*statisticsV2App.Run, error)
}

type StatisticsSyncOrgResult struct {
	OrgID    int64
	V1Status string
	V2Status string
}

type StatisticsSyncSummary struct {
	VersionMode string
	Orgs        []StatisticsSyncOrgResult
	Succeeded   int
	Failed      int
}

type StatisticsSyncPartialError struct{ Summary StatisticsSyncSummary }

func (e *StatisticsSyncPartialError) Error() string {
	return fmt.Sprintf("statistics sync partially failed: mode=%s succeeded=%d failed=%d", e.Summary.VersionMode, e.Summary.Succeeded, e.Summary.Failed)
}

// StatisticsSyncRunner executes nightly statistics sync inside apiserver.
type StatisticsSyncRunner struct {
	opts              *apiserveroptions.StatisticsSyncOptions
	syncService       statisticsSyncService
	v2Coordinator     statisticsV2Coordinator
	warmupCoordinator statisticsApp.WarmupCoordinator
	leader            leaderLeaseRunner
	clock             DailyClock
	now               func() time.Time
}

// NewStatisticsSyncRunner creates the statistics sync scheduler runner.
func NewStatisticsSyncRunner(
	opts *apiserveroptions.StatisticsSyncOptions,
	syncService statisticsSyncService,
	warmupCoordinator statisticsApp.WarmupCoordinator,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	v2Coordinator ...statisticsV2Coordinator,
) *StatisticsSyncRunner {
	runner := newStatisticsSyncRunnerWithHooks(
		opts,
		syncService,
		warmupCoordinator,
		lockManager,
		lockBuilder,
		func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
	if runner != nil && len(v2Coordinator) > 0 {
		runner.v2Coordinator = v2Coordinator[0]
	}
	return runner
}

func newStatisticsSyncRunnerWithHooks(
	opts *apiserveroptions.StatisticsSyncOptions,
	syncService statisticsSyncService,
	warmupCoordinator statisticsApp.WarmupCoordinator,
	lockManager locklease.Manager,
	lockBuilder *keyspace.Builder,
	acquireLock func(ctx context.Context, spec locklease.Spec, key string, ttl time.Duration) (*locklease.Lease, bool, error),
	releaseLock func(ctx context.Context, spec locklease.Spec, key string, lease *locklease.Lease) error,
) *StatisticsSyncRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	mode := normalizeStatisticsVersionMode(opts.VersionMode)
	if statisticsV1Enabled(mode) && syncService == nil {
		log.Warnf("statistics sync scheduler not started (module or sync service unavailable)")
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
	if opts.RepairWindowDays <= 0 {
		log.Warnf("statistics sync scheduler not started (repair_window_days must be greater than 0)")
		return nil
	}
	if opts.LockKey == "" {
		log.Warnf("statistics sync scheduler not started (lock_key is empty)")
		return nil
	}
	if opts.LockTTL <= 0 {
		log.Warnf("statistics sync scheduler not started (lock_ttl must be greater than 0)")
		return nil
	}
	if lockManager == nil {
		observability.ObserveLockDegraded("statistics_sync_leader", "redis_unavailable")
		log.Warnf("statistics sync scheduler not started (HA lock unavailable: redis client unavailable)")
		return nil
	}
	if acquireLock == nil || releaseLock == nil {
		log.Warnf("statistics sync scheduler not started (lock hooks unavailable)")
		return nil
	}

	return &StatisticsSyncRunner{
		opts:              opts,
		syncService:       syncService,
		warmupCoordinator: warmupCoordinator,
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

// Name returns the runner name.
func (r *StatisticsSyncRunner) Name() string {
	return "statistics_sync"
}

// Start starts the nightly statistics sync loop.
func (r *StatisticsSyncRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}

	lockKey := r.lockKey()
	log.Infof("statistics sync scheduler started (org_ids=%v, version_mode=%s, run_at=%s, repair_window_days=%d, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, normalizeStatisticsVersionMode(r.opts.VersionMode), r.opts.RunAt, r.opts.RepairWindowDays, lockKey, r.opts.LockTTL)

	go func() {
		for {
			now := r.now().In(statisticsV2Domain.Shanghai)
			nextRun := NextDailyRun(now, r.clock.Hour, r.clock.Minute)
			timer := time.NewTimer(time.Until(nextRun))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			r.executeTick(ctx)
		}
	}()
}

func (r *StatisticsSyncRunner) executeTick(ctx context.Context) {
	if err := r.runOnce(ctx); err != nil {
		log.Warnf("statistics sync scheduler tick failed: %v", err)
	}
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
		mode := normalizeStatisticsVersionMode(r.opts.VersionMode)
		summary := StatisticsSyncSummary{VersionMode: mode, Orgs: make([]StatisticsSyncOrgResult, 0, len(r.opts.OrgIDs))}
		for _, orgID := range r.opts.OrgIDs {
			start, end := statisticsSyncRepairWindow(r.now().In(statisticsV2Domain.Shanghai), r.opts.RepairWindowDays)
			orgResult := StatisticsSyncOrgResult{OrgID: orgID, V1Status: "disabled", V2Status: "disabled"}
			orgFailed := false
			if statisticsV1Enabled(mode) {
				if err := r.runV1Org(ctx, orgID, start, end); err != nil {
					orgResult.V1Status = "failed"
					orgFailed = true
					log.Warnf("statistics v1 nightly sync failed (org=%d): %v", orgID, err)
				} else {
					orgResult.V1Status = "succeeded"
				}
				observeStatisticsSchedulerOrg("v1", orgResult.V1Status)
			}
			if statisticsV2Enabled(mode) {
				if r.v2Coordinator == nil {
					orgResult.V2Status = "unavailable"
					orgFailed = true
					log.Warnf("statistics v2 nightly sync unavailable (org=%d)", orgID)
				} else {
					toDate := end.AddDate(0, 0, -1)
					run, err := r.v2Coordinator.Run(ctx, statisticsV2App.RunRequest{OrgID: orgID, FromDate: start, ToDate: toDate, Reason: "nightly shadow statistics v2", TriggerType: "scheduled", Mode: statisticsV2Domain.RunModePublish})
					if err != nil {
						orgResult.V2Status = "failed"
						if run != nil {
							orgResult.V2Status = string(run.Status)
						}
						orgFailed = true
						log.Warnf("statistics v2 nightly run failed (org=%d,status=%s): %v", orgID, orgResult.V2Status, err)
					} else {
						orgResult.V2Status = string(run.Status)
					}
				}
				observeStatisticsSchedulerOrg("v2", orgResult.V2Status)
			}
			if orgFailed {
				summary.Failed++
			} else {
				summary.Succeeded++
			}
			summary.Orgs = append(summary.Orgs, orgResult)
		}
		if summary.Failed > 0 {
			return &StatisticsSyncPartialError{Summary: summary}
		}
		return nil
	})
}

func (r *StatisticsSyncRunner) runV1Org(ctx context.Context, orgID int64, start, end time.Time) error {
	dailyOpts := statisticsApp.SyncDailyOptions{StartDate: &start, EndDate: &end}
	if err := r.syncService.SyncDailyStatistics(ctx, orgID, dailyOpts); err != nil {
		return fmt.Errorf("daily: %w", err)
	}
	if err := r.syncService.SyncOrgSnapshotStatistics(ctx, orgID); err != nil {
		return fmt.Errorf("org_snapshot: %w", err)
	}
	if err := r.syncService.SyncPlanStatistics(ctx, orgID); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	if r.warmupCoordinator != nil {
		if err := r.warmupCoordinator.HandleStatisticsSync(ctx, orgID); err != nil {
			return fmt.Errorf("cache_warmup: %w", err)
		}
	}
	return nil
}

func normalizeStatisticsVersionMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "shadow"
	}
	return mode
}

func statisticsV1Enabled(mode string) bool { return mode == "v1" || mode == "shadow" }
func statisticsV2Enabled(mode string) bool { return mode == "v2" || mode == "shadow" }

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
