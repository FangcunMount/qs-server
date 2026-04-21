package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
)

type statisticsSyncService interface {
	SyncDailyStatistics(ctx context.Context, orgID int64, opts statisticsApp.SyncDailyOptions) error
	SyncAccumulatedStatistics(ctx context.Context, orgID int64) error
	SyncPlanStatistics(ctx context.Context, orgID int64) error
}

// StatisticsSyncRunner executes nightly statistics sync inside apiserver.
type StatisticsSyncRunner struct {
	opts              *apiserveroptions.StatisticsSyncOptions
	syncService       statisticsSyncService
	warmupCoordinator cachegov.Coordinator
	lockManager       *redislock.Manager
	lockBuilder       *rediskey.Builder
	clock             DailyClock
	now               func() time.Time
	acquireLock       func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error)
	releaseLock       func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error
}

// NewStatisticsSyncRunner creates the statistics sync scheduler runner.
func NewStatisticsSyncRunner(
	opts *apiserveroptions.StatisticsSyncOptions,
	syncService statisticsSyncService,
	warmupCoordinator cachegov.Coordinator,
	lockManager *redislock.Manager,
	lockBuilder *rediskey.Builder,
) *StatisticsSyncRunner {
	return newStatisticsSyncRunnerWithHooks(
		opts,
		syncService,
		warmupCoordinator,
		lockManager,
		lockBuilder,
		func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error) {
			return lockManager.AcquireSpec(ctx, spec, key, ttl)
		},
		func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error {
			return lockManager.ReleaseSpec(ctx, spec, key, lease)
		},
	)
}

func newStatisticsSyncRunnerWithHooks(
	opts *apiserveroptions.StatisticsSyncOptions,
	syncService statisticsSyncService,
	warmupCoordinator cachegov.Coordinator,
	lockManager *redislock.Manager,
	lockBuilder *rediskey.Builder,
	acquireLock func(ctx context.Context, spec redislock.Spec, key string, ttl time.Duration) (*redislock.Lease, bool, error),
	releaseLock func(ctx context.Context, spec redislock.Spec, key string, lease *redislock.Lease) error,
) *StatisticsSyncRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if syncService == nil {
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
		cacheobservability.ObserveLockDegraded("statistics_sync_leader", "redis_unavailable")
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
		lockManager:       lockManager,
		lockBuilder:       lockBuilder,
		clock:             clock,
		now:               time.Now,
		acquireLock:       acquireLock,
		releaseLock:       releaseLock,
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
	log.Infof("statistics sync scheduler started (org_ids=%v, run_at=%s, repair_window_days=%d, lock_key=%s, lock_ttl=%s)",
		r.opts.OrgIDs, r.opts.RunAt, r.opts.RepairWindowDays, lockKey, r.opts.LockTTL)

	go func() {
		for {
			now := r.now().In(time.Local)
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
	lockSpec := redislock.Specs.StatisticsSyncLeader
	lockKey := r.lockKey()

	lease, acquired, err := r.acquireLock(ctx, lockSpec, r.opts.LockKey, r.opts.LockTTL)
	if err != nil {
		return fmt.Errorf("failed to acquire statistics sync scheduler lock: %w", err)
	}
	if !acquired {
		log.Debugf("statistics sync scheduler tick skipped (lock_key=%s, reason=lock_not_acquired)", lockKey)
		return nil
	}

	defer func() {
		if err := r.releaseLock(context.Background(), lockSpec, r.opts.LockKey, lease); err != nil {
			log.Warnf("failed to release statistics sync scheduler lock (lock_key=%s): %v", lockKey, err)
		}
	}()

	for _, orgID := range r.opts.OrgIDs {
		start, end := statisticsSyncRepairWindow(r.now().In(time.Local), r.opts.RepairWindowDays)
		dailyOpts := statisticsApp.SyncDailyOptions{StartDate: &start, EndDate: &end}
		if err := r.syncService.SyncDailyStatistics(ctx, orgID, dailyOpts); err != nil {
			log.Warnf("statistics nightly daily sync failed (org=%d): %v", orgID, err)
			continue
		}
		if err := r.syncService.SyncAccumulatedStatistics(ctx, orgID); err != nil {
			log.Warnf("statistics nightly accumulated sync failed (org=%d): %v", orgID, err)
			continue
		}
		if err := r.syncService.SyncPlanStatistics(ctx, orgID); err != nil {
			log.Warnf("statistics nightly plan sync failed (org=%d): %v", orgID, err)
			continue
		}
		if r.warmupCoordinator != nil {
			if err := r.warmupCoordinator.HandleStatisticsSync(ctx, orgID); err != nil {
				log.Warnf("statistics nightly cache warmup failed (org=%d): %v", orgID, err)
			}
		}
	}

	return nil
}

func (r *StatisticsSyncRunner) lockKey() string {
	if r == nil {
		return ""
	}
	if r.lockBuilder == nil {
		r.lockBuilder = rediskey.NewBuilder()
	}
	return r.lockBuilder.BuildLockKey(r.opts.LockKey)
}

func statisticsSyncRepairWindow(now time.Time, repairWindowDays int) (time.Time, time.Time) {
	if repairWindowDays <= 0 {
		repairWindowDays = 7
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -repairWindowDays), todayStart
}
