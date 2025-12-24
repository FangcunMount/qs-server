package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"gorm.io/gorm"
)

// systemStatisticsService 系统整体统计服务实现
type systemStatisticsService struct {
	db         *gorm.DB
	repo       *statisticsInfra.StatisticsRepository
	cache      *statisticsCache.StatisticsCache
	aggregator *statistics.Aggregator
}

// NewSystemStatisticsService 创建系统整体统计服务
func NewSystemStatisticsService(
	db *gorm.DB,
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
) SystemStatisticsService {
	return &systemStatisticsService{
		db:         db,
		repo:       repo,
		cache:      cache,
		aggregator: statistics.NewAggregator(),
	}
}

// GetSystemStatistics 获取系统整体统计
func (s *systemStatisticsService) GetSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取系统整体统计", "org_id", orgID)

	// 1. 优先从Redis缓存查询（查询结果缓存）
	if s.cache != nil {
		cacheKey := fmt.Sprintf("system:%d", orgID)
		cached, err := s.cache.GetQueryCache(ctx, cacheKey)
		if err == nil && cached != "" {
			var stats statistics.SystemStatistics
			if err := json.Unmarshal([]byte(cached), &stats); err == nil {
				l.Debugw("从Redis缓存获取系统统计", "cache_key", cacheKey)
				return &stats, nil
			}
		}
	}

	// 2. 其次从MySQL统计表查询（系统统计使用"system"作为statistic_key）
	if s.repo != nil {
		po, err := s.repo.GetAccumulatedStatistics(ctx, orgID, statistics.StatisticTypeSystem, "system")
		if err == nil && po != nil {
			// 系统统计需要从多个表聚合，暂时降级到原始表聚合
			// TODO: 完善系统统计的预聚合逻辑
		}
	}

	// 3. 最后从原始表实时聚合
	l.Debugw("从原始表实时聚合系统统计")
	result := &statistics.SystemStatistics{
		OrgID:                        orgID,
		AssessmentStatusDistribution: make(map[string]int64),
		AssessmentTrend:              []statistics.DailyCount{},
	}

	// 从 assessments 表聚合
	var assessmentCount int64
	if err := s.db.WithContext(ctx).
		Model(&struct {
			ID uint64 `gorm:"column:id"`
		}{}).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&assessmentCount).Error; err != nil {
		return nil, err
	}
	result.AssessmentCount = assessmentCount

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
		return nil, err
	}

	for _, sc := range statusCounts {
		result.AssessmentStatusDistribution[sc.Status] = sc.Count
	}

	// 今日新增
	today := time.Now().Format("2006-01-02")
	var todayCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND DATE(created_at) = ? AND deleted_at IS NULL", orgID, today).
		Count(&todayCount).Error; err != nil {
		return nil, err
	}
	result.TodayNewAssessments = todayCount

	// 近30天趋势（简化实现，实际应该从 statistics_daily 表查询）
	result.AssessmentTrend = []statistics.DailyCount{}

	return result, nil
}
