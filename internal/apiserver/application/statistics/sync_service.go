package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"gorm.io/gorm"
)

const defaultStatisticsRepairWindowDays = 7
const statisticsSyncLockTTL = 30 * time.Minute

// syncService 统计同步服务实现。
// 写侧只依赖 MySQL，把统计表视为可重建的物化视图。
type syncService struct {
	db               *gorm.DB
	repairWindowDays int
	lockManager      *redislock.Manager
}

// NewSyncService 创建统计同步服务。
func NewSyncService(
	db *gorm.DB,
	repairWindowDays int,
	lockManager *redislock.Manager,
) StatisticsSyncService {
	if repairWindowDays <= 0 {
		repairWindowDays = defaultStatisticsRepairWindowDays
	}
	return &syncService{
		db:               db,
		repairWindowDays: repairWindowDays,
		lockManager:      lockManager,
	}
}

// SyncDailyStatistics 同步每日统计（MySQL 原始表 → statistics_daily）。
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
	if err := s.withRedisLock(ctx, lockName, func(lockCtx context.Context) error {
		return s.db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`DELETE FROM statistics_daily
				  WHERE org_id = ? AND statistic_type IN ('questionnaire', 'system')
				    AND stat_date >= ? AND stat_date < ?`,
				orgID, startDate, endDate,
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				`INSERT INTO statistics_daily (
					org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
				)
				SELECT ?, 'questionnaire', agg.statistic_key, agg.stat_date, agg.submission_count, agg.completion_count
				FROM (
					SELECT raw.statistic_key, raw.stat_date,
					       SUM(raw.submission_count) AS submission_count,
					       SUM(raw.completion_count) AS completion_count
					FROM (
						SELECT questionnaire_code AS statistic_key,
						       DATE(created_at) AS stat_date,
						       1 AS submission_count,
						       0 AS completion_count
						FROM assessment
						WHERE org_id = ? AND deleted_at IS NULL
						  AND questionnaire_code <> ''
						  AND created_at >= ? AND created_at < ?
						UNION ALL
						SELECT questionnaire_code AS statistic_key,
						       DATE(interpreted_at) AS stat_date,
						       0 AS submission_count,
						       1 AS completion_count
						FROM assessment
						WHERE org_id = ? AND deleted_at IS NULL
						  AND questionnaire_code <> ''
						  AND interpreted_at IS NOT NULL
						  AND interpreted_at >= ? AND interpreted_at < ?
					) raw
					GROUP BY raw.statistic_key, raw.stat_date
				) agg`,
				orgID,
				orgID, startDate, endDate,
				orgID, startDate, endDate,
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				`INSERT INTO statistics_daily (
					org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
				)
				SELECT ?, 'system', 'system', agg.stat_date, agg.submission_count, agg.completion_count
				FROM (
					SELECT raw.stat_date,
					       SUM(raw.submission_count) AS submission_count,
					       SUM(raw.completion_count) AS completion_count
					FROM (
						SELECT DATE(created_at) AS stat_date,
						       1 AS submission_count,
						       0 AS completion_count
						FROM assessment
						WHERE org_id = ? AND deleted_at IS NULL
						  AND created_at >= ? AND created_at < ?
						UNION ALL
						SELECT DATE(interpreted_at) AS stat_date,
						       0 AS submission_count,
						       1 AS completion_count
						FROM assessment
						WHERE org_id = ? AND deleted_at IS NULL
						  AND interpreted_at IS NOT NULL
						  AND interpreted_at >= ? AND interpreted_at < ?
					) raw
					GROUP BY raw.stat_date
				) agg`,
				orgID,
				orgID, startDate, endDate,
				orgID, startDate, endDate,
			).Error; err != nil {
				return err
			}

			return nil
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

// SyncAccumulatedStatistics 从 statistics_daily / 原始表重建累计统计。
func (s *syncService) SyncAccumulatedStatistics(ctx context.Context, orgID int64) error {
	l := logger.L(ctx)
	l.Infow("开始重建累计统计", "action", "sync_accumulated_statistics", "org_id", orgID)
	if orgID <= 0 {
		l.Warnw("无效的 org_id，跳过累计统计同步", "org_id", orgID)
		return nil
	}

	todayStart, _ := currentDayBounds(time.Now().In(time.Local))
	lockName := fmt.Sprintf("statistics:accumulated:%d:%s", orgID, todayStart.Format("2006-01-02"))
	if err := s.withRedisLock(ctx, lockName, func(lockCtx context.Context) error {
		return s.db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`DELETE FROM statistics_accumulated
				  WHERE org_id = ? AND statistic_type IN ('questionnaire', 'system')`,
				orgID,
			).Error; err != nil {
				return err
			}

			questionnaireRows, err := s.buildQuestionnaireAccumulatedRows(lockCtx, tx, orgID, todayStart)
			if err != nil {
				return err
			}
			if len(questionnaireRows) > 0 {
				if err := tx.CreateInBatches(questionnaireRows, 200).Error; err != nil {
					return err
				}
			}

			systemRow, err := s.buildSystemAccumulatedRow(lockCtx, tx, orgID, todayStart)
			if err != nil {
				return err
			}
			if err := tx.Create(systemRow).Error; err != nil {
				return err
			}

			return nil
		})
	}); err != nil {
		return err
	}

	l.Infow("累计统计重建完成", "action", "sync_accumulated_statistics", "org_id", orgID)
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
	if err := s.withRedisLock(ctx, lockName, func(lockCtx context.Context) error {
		return s.db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("DELETE FROM statistics_plan WHERE org_id = ?", orgID).Error; err != nil {
				return err
			}

			var planRows []*statisticsInfra.StatisticsPlanPO
			if err := tx.Raw(
				`SELECT
					p.org_id AS org_id,
					p.id AS plan_id,
					COUNT(t.id) AS total_tasks,
					COALESCE(SUM(CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END), 0) AS completed_tasks,
					COALESCE(SUM(CASE WHEN t.status IN ('pending', 'opened') THEN 1 ELSE 0 END), 0) AS pending_tasks,
					COALESCE(SUM(CASE WHEN t.status = 'expired' THEN 1 ELSE 0 END), 0) AS expired_tasks,
					COUNT(DISTINCT t.testee_id) AS enrolled_testees,
					COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.testee_id END) AS active_testees
				FROM assessment_plan p
				LEFT JOIN assessment_task t
				  ON t.org_id = p.org_id
				 AND t.plan_id = p.id
				 AND t.deleted_at IS NULL
				WHERE p.org_id = ? AND p.deleted_at IS NULL
				GROUP BY p.org_id, p.id`,
				orgID,
			).Scan(&planRows).Error; err != nil {
				return err
			}

			if len(planRows) == 0 {
				return nil
			}
			return tx.CreateInBatches(planRows, 200).Error
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

func (s *syncService) buildQuestionnaireAccumulatedRows(
	ctx context.Context,
	tx *gorm.DB,
	orgID int64,
	todayStart time.Time,
) ([]*statisticsInfra.StatisticsAccumulatedPO, error) {
	last7d := todayStart.AddDate(0, 0, -7)
	last15d := todayStart.AddDate(0, 0, -15)
	last30d := todayStart.AddDate(0, 0, -30)

	var aggregates []struct {
		StatisticKey       string
		TotalSubmissions   int64
		TotalCompletions   int64
		Last7dSubmissions  int64
		Last15dSubmissions int64
		Last30dSubmissions int64
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT
			statistic_key,
			COALESCE(SUM(submission_count), 0) AS total_submissions,
			COALESCE(SUM(completion_count), 0) AS total_completions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last7d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last15d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last30d_submissions
		FROM statistics_daily
		WHERE org_id = ? AND statistic_type = 'questionnaire'
		GROUP BY statistic_key`,
		last7d, last15d, last30d, orgID,
	).Scan(&aggregates).Error; err != nil {
		return nil, err
	}

	originDistribution := make(map[string]statisticsInfra.JSONField)
	var originRows []struct {
		QuestionnaireCode string
		OriginType        string
		Count             int64
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT questionnaire_code, origin_type, COUNT(*) AS count
		FROM assessment
		WHERE org_id = ? AND deleted_at IS NULL
		  AND questionnaire_code <> ''
		  AND created_at < ?
		GROUP BY questionnaire_code, origin_type`,
		orgID, todayStart,
	).Scan(&originRows).Error; err != nil {
		return nil, err
	}
	for _, row := range originRows {
		if originDistribution[row.QuestionnaireCode] == nil {
			originDistribution[row.QuestionnaireCode] = statisticsInfra.JSONField{}
		}
		originDistribution[row.QuestionnaireCode][row.OriginType] = row.Count
	}

	timeBounds := make(map[string]struct {
		FirstOccurredAt *time.Time
		LastOccurredAt  *time.Time
	})
	var timeRows []struct {
		QuestionnaireCode string
		FirstOccurredAt   *time.Time
		LastOccurredAt    *time.Time
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT questionnaire_code, MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at
		FROM assessment
		WHERE org_id = ? AND deleted_at IS NULL
		  AND questionnaire_code <> ''
		  AND created_at < ?
		GROUP BY questionnaire_code`,
		orgID, todayStart,
	).Scan(&timeRows).Error; err != nil {
		return nil, err
	}
	for _, row := range timeRows {
		timeBounds[row.QuestionnaireCode] = struct {
			FirstOccurredAt *time.Time
			LastOccurredAt  *time.Time
		}{FirstOccurredAt: row.FirstOccurredAt, LastOccurredAt: row.LastOccurredAt}
	}

	result := make([]*statisticsInfra.StatisticsAccumulatedPO, 0, len(aggregates))
	for _, aggregate := range aggregates {
		distribution := statisticsInfra.JSONField{}
		if origin := originDistribution[aggregate.StatisticKey]; len(origin) > 0 {
			distribution["origin"] = origin
		}
		bounds := timeBounds[aggregate.StatisticKey]
		result = append(result, &statisticsInfra.StatisticsAccumulatedPO{
			OrgID:              orgID,
			StatisticType:      "questionnaire",
			StatisticKey:       aggregate.StatisticKey,
			TotalSubmissions:   aggregate.TotalSubmissions,
			TotalCompletions:   aggregate.TotalCompletions,
			Last7dSubmissions:  aggregate.Last7dSubmissions,
			Last15dSubmissions: aggregate.Last15dSubmissions,
			Last30dSubmissions: aggregate.Last30dSubmissions,
			Distribution:       distribution,
			FirstOccurredAt:    bounds.FirstOccurredAt,
			LastOccurredAt:     bounds.LastOccurredAt,
		})
	}
	return result, nil
}

func (s *syncService) buildSystemAccumulatedRow(
	ctx context.Context,
	tx *gorm.DB,
	orgID int64,
	todayStart time.Time,
) (*statisticsInfra.StatisticsAccumulatedPO, error) {
	var assessmentCount int64
	if err := tx.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Count(&assessmentCount).Error; err != nil {
		return nil, err
	}

	var completionCount int64
	if err := tx.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at < ?", orgID, todayStart).
		Count(&completionCount).Error; err != nil {
		return nil, err
	}

	var testeeCount int64
	if err := tx.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Count(&testeeCount).Error; err != nil {
		return nil, err
	}

	statusDistribution := statisticsInfra.JSONField{}
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := tx.WithContext(ctx).
		Table("assessment").
		Select("status, COUNT(*) AS count").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, row := range statusRows {
		statusDistribution[row.Status] = row.Count
	}

	var timeInfo struct {
		FirstOccurredAt *time.Time
		LastOccurredAt  *time.Time
	}
	if err := tx.WithContext(ctx).
		Table("assessment").
		Select("MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Scan(&timeInfo).Error; err != nil {
		return nil, err
	}

	return &statisticsInfra.StatisticsAccumulatedPO{
		OrgID:            orgID,
		StatisticType:    "system",
		StatisticKey:     "system",
		TotalSubmissions: assessmentCount,
		TotalCompletions: completionCount,
		Distribution: statisticsInfra.JSONField{
			"status":       statusDistribution,
			"testee_count": testeeCount,
		},
		FirstOccurredAt: timeInfo.FirstOccurredAt,
		LastOccurredAt:  timeInfo.LastOccurredAt,
	}, nil
}

func (s *syncService) withRedisLock(ctx context.Context, lockName string, fn func(context.Context) error) error {
	if s.lockManager == nil {
		return fmt.Errorf("statistics sync redis lock manager is unavailable")
	}

	lease, acquired, err := s.lockManager.AcquireSpec(ctx, redislock.Specs.StatisticsSync, lockName, statisticsSyncLockTTL)
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("statistics sync lock busy: %s", lockName)
	}
	defer func() {
		_ = s.lockManager.ReleaseSpec(context.Background(), redislock.Specs.StatisticsSync, lockName, lease)
	}()

	return fn(ctx)
}

func normalizeLocalDay(value time.Time) time.Time {
	local := value.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
