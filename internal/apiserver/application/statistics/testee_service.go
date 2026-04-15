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

// testeeStatisticsService 受试者统计服务实现
type testeeStatisticsService struct {
	db         *gorm.DB
	repo       *statisticsInfra.StatisticsRepository
	cache      *statisticsCache.StatisticsCache
	aggregator *statistics.Aggregator
}

// NewTesteeStatisticsService 创建受试者统计服务
func NewTesteeStatisticsService(
	db *gorm.DB,
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
) TesteeStatisticsService {
	return &testeeStatisticsService{
		db:         db,
		repo:       repo,
		cache:      cache,
		aggregator: statistics.NewAggregator(),
	}
}

// GetTesteeStatistics 获取受试者统计
func (s *testeeStatisticsService) GetTesteeStatistics(
	ctx context.Context,
	orgID int64,
	testeeID uint64,
) (*statistics.TesteeStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取受试者统计", "org_id", orgID, "testee_id", testeeID)

	// 1. 优先从Redis缓存查询
	if s.cache != nil {
		cacheKey := fmt.Sprintf("testee:%d:%d", orgID, testeeID)
		cached, err := s.cache.GetQueryCache(ctx, cacheKey)
		if err == nil && cached != "" {
			var stats statistics.TesteeStatistics
			if err := json.Unmarshal([]byte(cached), &stats); err == nil {
				l.Debugw("从Redis缓存获取受试者统计", "cache_key", cacheKey)
				return &stats, nil
			}
		}
	}

	// 2. 从原始表实时聚合
	l.Debugw("从原始表实时聚合受试者统计")
	result := &statistics.TesteeStatistics{
		OrgID:            orgID,
		TesteeID:         testeeID,
		RiskDistribution: make(map[string]int64),
	}

	// 从 assessments 表聚合
	var totalAssessments int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Count(&totalAssessments).Error; err != nil {
		return nil, err
	}
	result.TotalAssessments = totalAssessments

	// 已完成测评数
	var completedAssessments int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND testee_id = ? AND status = 'interpreted' AND deleted_at IS NULL",
			orgID, testeeID).
		Count(&completedAssessments).Error; err != nil {
		return nil, err
	}
	result.CompletedAssessments = completedAssessments
	result.PendingAssessments = totalAssessments - completedAssessments

	// 风险分布
	var riskCounts []struct {
		RiskLevel string
		Count     int64
	}
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("risk_level, COUNT(*) as count").
		Where("org_id = ? AND testee_id = ? AND risk_level IS NOT NULL AND deleted_at IS NULL",
			orgID, testeeID).
		Group("risk_level").
		Scan(&riskCounts).Error; err != nil {
		return nil, err
	}

	for _, rc := range riskCounts {
		if rc.RiskLevel != "" {
			result.RiskDistribution[rc.RiskLevel] = rc.Count
		}
	}

	// 时间维度
	var timeInfo struct {
		FirstAssessmentDate *time.Time
		LastAssessmentDate  *time.Time
	}
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("MIN(created_at) as first_assessment_date, MAX(interpreted_at) as last_assessment_date").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Scan(&timeInfo).Error; err == nil {
		result.FirstAssessmentDate = timeInfo.FirstAssessmentDate
		result.LastAssessmentDate = timeInfo.LastAssessmentDate
	}

	// 缓存结果（TTL=5分钟）
	if s.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			cacheKey := fmt.Sprintf("testee:%d:%d", orgID, testeeID)
			if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute); err != nil {
				l.Warnw("写入受试者统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
			}
		} else {
			l.Warnw("序列化受试者统计结果失败", "error", err)
		}
	}

	return result, nil
}
