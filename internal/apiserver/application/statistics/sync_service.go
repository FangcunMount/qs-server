package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"gorm.io/gorm"
)

// syncService 统计同步服务实现
type syncService struct {
	repo  *statisticsInfra.StatisticsRepository
	cache *statisticsCache.StatisticsCache
	db    *gorm.DB
}

// NewSyncService 创建统计同步服务
func NewSyncService(
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
	db *gorm.DB,
) StatisticsSyncService {
	return &syncService{
		repo:  repo,
		cache: cache,
		db:    db,
	}
}

// SyncDailyStatistics 同步每日统计（Redis → MySQL）
func (s *syncService) SyncDailyStatistics(ctx context.Context) error {
	l := logger.L(ctx)
	l.Infow("开始同步每日统计", "action", "sync_daily_statistics")

	// 使用全局常量：org_id 固定为 1（单租户场景）
	orgIDs := []int64{DefaultOrgID}

	if len(orgIDs) == 0 {
		l.Infow("未找到任何org_id，跳过同步")
		return nil
	}

	statTypes := []statistics.StatisticType{
		statistics.StatisticTypeQuestionnaire,
		statistics.StatisticTypeTestee,
		statistics.StatisticTypePlan,
		statistics.StatisticTypeScreening,
	}

	totalSynced := 0
	for _, orgID := range orgIDs {
		for _, statType := range statTypes {
			// 扫描该org和type下的所有每日统计键
			keys, err := s.cache.ScanDailyKeys(ctx, orgID, statType)
			if err != nil {
				l.Errorw("扫描每日统计键失败",
					"org_id", orgID,
					"stat_type", statType,
					"error", err.Error(),
				)
				continue
			}

			// 解析键并同步
			for _, key := range keys {
				// 解析键格式：stats:daily:{org_id}:{type}:{key}:{date}
				// 例如：stats:daily:1:questionnaire:Q001:2025-01-20
				parts := parseDailyKey(key)
				if len(parts) != 6 {
					l.Warnw("每日统计键格式不正确，跳过",
						"key", key,
					)
					continue
				}

				statKey := parts[4]
				dateStr := parts[5]

				// 解析日期
				date, err := time.Parse("2006-01-02", dateStr)
				if err != nil {
					l.Warnw("日期格式不正确，跳过",
						"key", key,
						"date", dateStr,
						"error", err.Error(),
					)
					continue
				}

				// 从Redis读取每日统计
				submissionCount, completionCount, err := s.cache.GetDailyCount(ctx, orgID, statType, statKey, date)
				if err != nil {
					l.Errorw("读取每日统计失败",
						"key", key,
						"error", err.Error(),
					)
					continue
				}

				// 同步到MySQL
				po := &statisticsInfra.StatisticsDailyPO{
					OrgID:           orgID,
					StatisticType:   string(statType),
					StatisticKey:    statKey,
					StatDate:        date,
					SubmissionCount: submissionCount,
					CompletionCount: completionCount,
				}

				if err := s.repo.UpsertDailyStatistics(ctx, po); err != nil {
					l.Errorw("同步每日统计到MySQL失败",
						"key", key,
						"error", err.Error(),
					)
					continue
				}

				totalSynced++
			}
		}
	}

	l.Infow("每日统计同步完成",
		"action", "sync_daily_statistics",
		"total_synced", totalSynced,
	)

	return nil
}

// SyncAccumulatedStatistics 同步累计统计（Redis → MySQL）
func (s *syncService) SyncAccumulatedStatistics(ctx context.Context) error {
	l := logger.L(ctx)
	l.Infow("开始同步累计统计", "action", "sync_accumulated_statistics")

	// 使用全局常量：org_id 固定为 1（单租户场景）
	orgIDs := []int64{DefaultOrgID}

	if len(orgIDs) == 0 {
		l.Infow("未找到任何org_id，跳过同步")
		return nil
	}

	// 从每日统计聚合到累计统计

	statTypes := []statistics.StatisticType{
		statistics.StatisticTypeQuestionnaire,
		statistics.StatisticTypeTestee,
	}

	for _, orgID := range orgIDs {
		for _, statType := range statTypes {
			// 扫描该org和type下的所有统计键
			keys, err := s.cache.ScanDailyKeys(ctx, orgID, statType)
			if err != nil {
				l.Errorw("扫描统计键失败",
					"org_id", orgID,
					"stat_type", statType,
					"error", err.Error(),
				)
				continue
			}

			// 提取唯一的statKey
			statKeys := make(map[string]bool)
			for _, key := range keys {
				parts := parseDailyKey(key)
				if len(parts) == 6 {
					statKeys[parts[4]] = true
				}
			}

			// 对每个statKey进行聚合
			for statKey := range statKeys {
				if err := s.repo.AggregateDailyToAccumulated(ctx, orgID, statType, statKey); err != nil {
					l.Errorw("聚合累计统计失败",
						"org_id", orgID,
						"stat_type", statType,
						"stat_key", statKey,
						"error", err.Error(),
					)
					continue
				}
			}
		}

		// 同步系统统计（从原始表直接聚合）
		if err := s.syncSystemStatistics(ctx, orgID); err != nil {
			l.Errorw("同步系统统计失败",
				"org_id", orgID,
				"error", err.Error(),
			)
			// 继续处理其他组织，不中断
		}
	}

	l.Infow("累计统计同步完成", "action", "sync_accumulated_statistics")
	return nil
}

// SyncPlanStatistics 同步计划统计
func (s *syncService) SyncPlanStatistics(ctx context.Context) error {
	l := logger.L(ctx)
	l.Infow("开始同步计划统计", "action", "sync_plan_statistics")

	// 从assessment_plan表查询所有计划
	var plans []struct {
		OrgID  int64
		PlanID uint64
	}
	if err := s.db.WithContext(ctx).
		Table("assessment_plan").
		Select("org_id, id as plan_id").
		Where("deleted_at IS NULL").
		Scan(&plans).Error; err != nil {
		l.Errorw("查询计划列表失败", "error", err.Error())
		return err
	}

	// 对每个计划聚合任务统计
	for _, plan := range plans {
		var taskStats struct {
			TotalTasks      int64
			CompletedTasks  int64
			PendingTasks    int64
			ExpiredTasks    int64
			EnrolledTestees int64
			ActiveTestees   int64
		}

		if err := s.db.WithContext(ctx).
			Table("assessment_task").
			Select(`
				COUNT(*) as total_tasks,
				SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_tasks,
				SUM(CASE WHEN status IN ('pending', 'opened') THEN 1 ELSE 0 END) as pending_tasks,
				SUM(CASE WHEN status = 'expired' THEN 1 ELSE 0 END) as expired_tasks,
				COUNT(DISTINCT testee_id) as enrolled_testees,
				COUNT(DISTINCT CASE WHEN status = 'completed' THEN testee_id END) as active_testees
			`).
			Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", plan.OrgID, plan.PlanID).
			Scan(&taskStats).Error; err != nil {
			l.Errorw("聚合计划统计失败",
				"org_id", plan.OrgID,
				"plan_id", plan.PlanID,
				"error", err.Error(),
			)
			continue
		}

		// 同步到MySQL
		po := &statisticsInfra.StatisticsPlanPO{
			OrgID:           plan.OrgID,
			PlanID:          plan.PlanID,
			TotalTasks:      taskStats.TotalTasks,
			CompletedTasks:  taskStats.CompletedTasks,
			PendingTasks:    taskStats.PendingTasks,
			ExpiredTasks:    taskStats.ExpiredTasks,
			EnrolledTestees: taskStats.EnrolledTestees,
			ActiveTestees:   taskStats.ActiveTestees,
		}

		if err := s.repo.UpsertPlanStatistics(ctx, po); err != nil {
			l.Errorw("同步计划统计到MySQL失败",
				"org_id", plan.OrgID,
				"plan_id", plan.PlanID,
				"error", err.Error(),
			)
			continue
		}
	}

	l.Infow("计划统计同步完成", "action", "sync_plan_statistics", "plan_count", len(plans))
	return nil
}

// syncSystemStatistics 同步系统统计（从原始表聚合）
func (s *syncService) syncSystemStatistics(ctx context.Context, orgID int64) error {
	l := logger.L(ctx)
	l.Debugw("开始同步系统统计", "org_id", orgID)

	// 从 assessments 表聚合
	var assessmentCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&assessmentCount).Error; err != nil {
		return err
	}

	// 从 testees 表聚合
	var testeeCount int64
	if err := s.db.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&testeeCount).Error; err != nil {
		return err
	}

	// 按状态统计
	var statusCounts []struct {
		Status string
		Count  int64
	}
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("status, COUNT(*) as count").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return err
	}

	// 构建状态分布
	statusDistribution := make(map[string]interface{})
	for _, sc := range statusCounts {
		statusDistribution[sc.Status] = sc.Count
	}

	// 今日新增
	today := time.Now().Format("2006-01-02")
	var todayNewAssessments int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND DATE(created_at) = ? AND deleted_at IS NULL", orgID, today).
		Count(&todayNewAssessments).Error; err != nil {
		return err
	}

	var todayNewTestees int64
	if err := s.db.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND DATE(created_at) = ? AND deleted_at IS NULL", orgID, today).
		Count(&todayNewTestees).Error; err != nil {
		return err
	}

	// 构建 Distribution JSON 字段
	distribution := statisticsInfra.JSONField{
		"status":                statusDistribution,
		"testee_count":          testeeCount,
		"today_new_assessments": todayNewAssessments,
		"today_new_testees":     todayNewTestees,
		// 问卷和答卷数量需要从 MongoDB 查询，暂时不包含
		// "questionnaire_count":     0,
		// "answer_sheet_count":       0,
		// "today_new_answer_sheets":  0,
	}

	// 同步到 MySQL
	po := &statisticsInfra.StatisticsAccumulatedPO{
		OrgID:            orgID,
		StatisticType:    string(statistics.StatisticTypeSystem),
		StatisticKey:     "system",
		TotalSubmissions: assessmentCount,
		TotalCompletions: 0, // 系统统计不区分提交和完成
		Distribution:     distribution,
	}

	// 获取首次和最后发生时间
	var timeInfo struct {
		FirstOccurredAt *time.Time
		LastOccurredAt  *time.Time
	}
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("MIN(created_at) as first_occurred_at, MAX(created_at) as last_occurred_at").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Scan(&timeInfo).Error; err == nil {
		po.FirstOccurredAt = timeInfo.FirstOccurredAt
		po.LastOccurredAt = timeInfo.LastOccurredAt
	}

	if err := s.repo.UpsertAccumulatedStatistics(ctx, po); err != nil {
		return err
	}

	l.Debugw("系统统计同步完成", "org_id", orgID)
	return nil
}
