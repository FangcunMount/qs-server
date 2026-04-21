package scheduler

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
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
	clock             DailyClock
	now               func() time.Time
}

// NewStatisticsSyncRunner creates the statistics sync scheduler runner.
func NewStatisticsSyncRunner(
	opts *apiserveroptions.StatisticsSyncOptions,
	syncService statisticsSyncService,
	warmupCoordinator cachegov.Coordinator,
) *StatisticsSyncRunner {
	if opts == nil || !opts.Enable {
		return nil
	}
	if syncService == nil {
		log.Warnf("statistics sync scheduler not started (module or sync service unavailable)")
		return nil
	}

	clock, err := ParseDailyClock(opts.RunAt)
	if err != nil {
		log.Warnf("statistics sync scheduler disabled: invalid run_at %q: %v", opts.RunAt, err)
		return nil
	}

	return &StatisticsSyncRunner{
		opts:              opts,
		syncService:       syncService,
		warmupCoordinator: warmupCoordinator,
		clock:             clock,
		now:               time.Now,
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

	log.Infof("statistics sync scheduler started (org_ids=%v, run_at=%s, repair_window_days=%d)",
		r.opts.OrgIDs, r.opts.RunAt, r.opts.RepairWindowDays)

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

			for _, orgID := range r.opts.OrgIDs {
				orgCtx := context.Background()
				start, end := statisticsSyncRepairWindow(r.now().In(time.Local), r.opts.RepairWindowDays)
				dailyOpts := statisticsApp.SyncDailyOptions{StartDate: &start, EndDate: &end}
				if err := r.syncService.SyncDailyStatistics(orgCtx, orgID, dailyOpts); err != nil {
					log.Warnf("statistics nightly daily sync failed (org=%d): %v", orgID, err)
					continue
				}
				if err := r.syncService.SyncAccumulatedStatistics(orgCtx, orgID); err != nil {
					log.Warnf("statistics nightly accumulated sync failed (org=%d): %v", orgID, err)
					continue
				}
				if err := r.syncService.SyncPlanStatistics(orgCtx, orgID); err != nil {
					log.Warnf("statistics nightly plan sync failed (org=%d): %v", orgID, err)
					continue
				}
				if r.warmupCoordinator != nil {
					if err := r.warmupCoordinator.HandleStatisticsSync(orgCtx, orgID); err != nil {
						log.Warnf("statistics nightly cache warmup failed (org=%d): %v", orgID, err)
					}
				}
			}
		}
	}()
}

func statisticsSyncRepairWindow(now time.Time, repairWindowDays int) (time.Time, time.Time) {
	if repairWindowDays <= 0 {
		repairWindowDays = 7
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -repairWindowDays), todayStart
}
