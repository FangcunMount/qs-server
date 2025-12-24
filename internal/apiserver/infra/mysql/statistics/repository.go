package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// StatisticsRepository 统计仓储实现
type StatisticsRepository struct {
	mysql.BaseRepository[*StatisticsAccumulatedPO]
	db *gorm.DB
}

// NewStatisticsRepository 创建统计仓储
func NewStatisticsRepository(db *gorm.DB) *StatisticsRepository {
	return &StatisticsRepository{
		BaseRepository: mysql.NewBaseRepository[*StatisticsAccumulatedPO](db),
		db:             db,
	}
}

// ==================== 累计统计查询 ====================

// GetAccumulatedStatistics 获取累计统计
func (r *StatisticsRepository) GetAccumulatedStatistics(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
) (*StatisticsAccumulatedPO, error) {
	var po StatisticsAccumulatedPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ?", orgID, statType, statKey).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &po, nil
}

// UpsertAccumulatedStatistics 更新或插入累计统计
func (r *StatisticsRepository) UpsertAccumulatedStatistics(
	ctx context.Context,
	po *StatisticsAccumulatedPO,
) error {
	return r.WithContext(ctx).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ?",
			po.OrgID, po.StatisticType, po.StatisticKey).
		Assign(*po).
		FirstOrCreate(po).Error
}

// ==================== 每日统计查询 ====================

// GetDailyStatistics 获取每日统计
func (r *StatisticsRepository) GetDailyStatistics(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	startDate, endDate time.Time,
) ([]*StatisticsDailyPO, error) {
	var pos []*StatisticsDailyPO
	err := r.WithContext(ctx).
		Model(&StatisticsDailyPO{}).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ? AND stat_date >= ? AND stat_date <= ?",
			orgID, statType, statKey, startDate, endDate).
		Order("stat_date ASC").
		Find(&pos).Error

	return pos, err
}

// UpsertDailyStatistics 更新或插入每日统计
func (r *StatisticsRepository) UpsertDailyStatistics(
	ctx context.Context,
	po *StatisticsDailyPO,
) error {
	return r.WithContext(ctx).
		Model(&StatisticsDailyPO{}).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ? AND stat_date = ?",
			po.OrgID, po.StatisticType, po.StatisticKey, po.StatDate).
		Assign(*po).
		FirstOrCreate(po).Error
}

// ==================== 计划统计查询 ====================

// GetPlanStatistics 获取计划统计
func (r *StatisticsRepository) GetPlanStatistics(
	ctx context.Context,
	orgID int64,
	planID uint64,
) (*StatisticsPlanPO, error) {
	var po StatisticsPlanPO
	err := r.WithContext(ctx).
		Model(&StatisticsPlanPO{}).
		Where("org_id = ? AND plan_id = ?", orgID, planID).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &po, nil
}

// UpsertPlanStatistics 更新或插入计划统计
func (r *StatisticsRepository) UpsertPlanStatistics(
	ctx context.Context,
	po *StatisticsPlanPO,
) error {
	return r.WithContext(ctx).
		Model(&StatisticsPlanPO{}).
		Where("org_id = ? AND plan_id = ?", po.OrgID, po.PlanID).
		Assign(*po).
		FirstOrCreate(po).Error
}

// ==================== 聚合查询 ====================

// AggregateDailyToAccumulated 从每日统计聚合到累计统计
func (r *StatisticsRepository) AggregateDailyToAccumulated(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
) error {
	// 计算近7/15/30天的数据
	now := time.Now()
	last7d := now.AddDate(0, 0, -7)
	last15d := now.AddDate(0, 0, -15)
	last30d := now.AddDate(0, 0, -30)

	// 查询累计统计
	var accumulated StatisticsAccumulatedPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ?", orgID, statType, statKey).
		First(&accumulated).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	// 聚合每日统计
	var dailyStats []StatisticsDailyPO
	err = r.WithContext(ctx).
		Model(&StatisticsDailyPO{}).
		Where("org_id = ? AND statistic_type = ? AND statistic_key = ?",
			orgID, statType, statKey).
		Find(&dailyStats).Error

	if err != nil {
		return err
	}

	// 计算累计值
	totalSubmissions := int64(0)
	totalCompletions := int64(0)
	last7dCount := int64(0)
	last15dCount := int64(0)
	last30dCount := int64(0)

	for _, daily := range dailyStats {
		totalSubmissions += daily.SubmissionCount
		totalCompletions += daily.CompletionCount

		if daily.StatDate.After(last7d) || daily.StatDate.Equal(last7d) {
			last7dCount += daily.SubmissionCount
		}
		if daily.StatDate.After(last15d) || daily.StatDate.Equal(last15d) {
			last15dCount += daily.SubmissionCount
		}
		if daily.StatDate.After(last30d) || daily.StatDate.Equal(last30d) {
			last30dCount += daily.SubmissionCount
		}
	}

	// 更新累计统计
	accumulated.OrgID = orgID
	accumulated.StatisticType = string(statType)
	accumulated.StatisticKey = statKey
	accumulated.TotalSubmissions = totalSubmissions
	accumulated.TotalCompletions = totalCompletions
	accumulated.Last7dSubmissions = last7dCount
	accumulated.Last15dSubmissions = last15dCount
	accumulated.Last30dSubmissions = last30dCount

	return r.UpsertAccumulatedStatistics(ctx, &accumulated)
}
