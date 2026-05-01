package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
)

const defaultStatisticsRepairWindowDays = 7
const statisticsSyncLockTTL = 30 * time.Minute

// syncService 统计同步服务实现。
// application 只负责锁、时间窗口和事务编排；统计表重建 SQL 属于 infra writer。
type syncService struct {
	uow              apptransaction.Runner
	writer           StatisticsRebuildWriter
	repairWindowDays int
	lockManager      locklease.Manager
}

func NewSyncService(
	runner apptransaction.Runner,
	writer StatisticsRebuildWriter,
	repairWindowDays int,
	lockManager locklease.Manager,
) StatisticsSyncService {
	return NewSyncServiceWithTransactionRunner(runner, writer, repairWindowDays, lockManager)
}

func NewSyncServiceWithTransactionRunner(
	runner apptransaction.Runner,
	writer StatisticsRebuildWriter,
	repairWindowDays int,
	lockManager locklease.Manager,
) StatisticsSyncService {
	if repairWindowDays <= 0 {
		repairWindowDays = defaultStatisticsRepairWindowDays
	}
	return &syncService{
		uow:              runner,
		writer:           writer,
		repairWindowDays: repairWindowDays,
		lockManager:      lockManager,
	}
}

// SyncDailyStatistics 同步每日统计（MySQL 原始表 → consolidated statistics daily read models）。
func (s *syncService) SyncDailyStatistics(ctx context.Context, orgID int64, opts SyncDailyOptions) error {
	l := logger.L(ctx)
	l.Infow("开始重建每日统计", "action", "sync_daily_statistics", "org_id", orgID)

	if orgID <= 0 {
		l.Warnw("无效的 org_id，跳过每日统计同步", "org_id", orgID)
		return nil
	}

	startDate, endDate, err := s.normalizeDailyWindow(time.Now().In(time.Local), opts)
	if err != nil {
		return err
	}
	if !startDate.Before(endDate) {
		l.Warnw("每日统计窗口为空，跳过", "org_id", orgID, "start_date", startDate, "end_date", endDate)
		return nil
	}

	lockName := fmt.Sprintf("statistics:daily:%d:%s:%s", orgID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err := s.withLockLease(ctx, lockName, func(lockCtx context.Context) error {
		return s.uow.WithinTransaction(lockCtx, func(txCtx context.Context) error {
			return s.writer.RebuildDailyStatistics(txCtx, orgID, startDate, endDate)
		})
	}); err != nil {
		return err
	}

	l.Infow("每日统计重建完成",
		"action", "sync_daily_statistics",
		"org_id", orgID,
		"start_date", startDate.Format("2006-01-02"),
		"end_date", endDate.Format("2006-01-02"),
	)
	return nil
}

// SyncOrgSnapshotStatistics 刷新机构级统计快照。
func (s *syncService) SyncOrgSnapshotStatistics(ctx context.Context, orgID int64) error {
	l := logger.L(ctx)
	l.Infow("开始重建机构统计快照", "action", "sync_org_snapshot_statistics", "org_id", orgID)
	if orgID <= 0 {
		l.Warnw("无效的 org_id，跳过机构统计快照同步", "org_id", orgID)
		return nil
	}

	todayStart, _ := currentDayBounds(time.Now().In(time.Local))
	lockName := fmt.Sprintf("statistics:org_snapshot:%d:%s", orgID, todayStart.Format("2006-01-02"))
	if err := s.withLockLease(ctx, lockName, func(lockCtx context.Context) error {
		return s.uow.WithinTransaction(lockCtx, func(txCtx context.Context) error {
			return s.writer.RebuildOrgSnapshotStatistics(txCtx, orgID, todayStart)
		})
	}); err != nil {
		return err
	}

	l.Infow("机构统计快照重建完成", "action", "sync_org_snapshot_statistics", "org_id", orgID)
	return nil
}

// SyncPlanStatistics 从 assessment_task 重建计划统计。
func (s *syncService) SyncPlanStatistics(ctx context.Context, orgID int64) error {
	l := logger.L(ctx)
	l.Infow("开始重建计划统计", "action", "sync_plan_statistics", "org_id", orgID)
	if orgID <= 0 {
		l.Warnw("无效的 org_id，跳过计划统计同步", "org_id", orgID)
		return nil
	}

	todayStart, _ := currentDayBounds(time.Now().In(time.Local))
	lockName := fmt.Sprintf("statistics:plan:%d:%s", orgID, todayStart.Format("2006-01-02"))
	if err := s.withLockLease(ctx, lockName, func(lockCtx context.Context) error {
		return s.uow.WithinTransaction(lockCtx, func(txCtx context.Context) error {
			return s.writer.RebuildPlanStatistics(txCtx, orgID)
		})
	}); err != nil {
		return err
	}

	l.Infow("计划统计重建完成", "action", "sync_plan_statistics", "org_id", orgID)
	return nil
}

func (s *syncService) normalizeDailyWindow(now time.Time, opts SyncDailyOptions) (time.Time, time.Time, error) {
	if opts.StartDate == nil && opts.EndDate == nil {
		todayStart, _ := currentDayBounds(now)
		return todayStart.AddDate(0, 0, -s.repairWindowDays), todayStart, nil
	}
	if opts.StartDate == nil || opts.EndDate == nil {
		return time.Time{}, time.Time{}, fmt.Errorf("statistics sync date range requires both start and end dates")
	}

	start := normalizeLocalDay(*opts.StartDate)
	end := normalizeLocalDay(*opts.EndDate)
	if !start.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("statistics sync date range must satisfy start < end")
	}
	return start, end, nil
}

func (s *syncService) withLockLease(ctx context.Context, lockName string, fn func(context.Context) error) error {
	if s.lockManager == nil {
		return fmt.Errorf("statistics sync redis lock manager is unavailable")
	}
	if s.uow == nil {
		return fmt.Errorf("statistics sync transaction runner is unavailable")
	}
	if s.writer == nil {
		return fmt.Errorf("statistics sync writer is unavailable")
	}

	lease, acquired, err := s.lockManager.AcquireSpec(ctx, locklease.Specs.StatisticsSync, lockName, statisticsSyncLockTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("statistics sync lock busy: %s", lockName)
	}
	defer func() {
		_ = s.lockManager.ReleaseSpec(context.Background(), locklease.Specs.StatisticsSync, lockName, lease)
	}()

	return fn(ctx)
}

func normalizeLocalDay(value time.Time) time.Time {
	local := value.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
