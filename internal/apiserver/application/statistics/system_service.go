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
			// 转换为领域对象
			stats := s.convertAccumulatedPOToSystemStatistics(po, orgID)

			// 缓存结果（TTL=5分钟）
			if s.cache != nil {
				if data, err := json.Marshal(stats); err == nil {
					cacheKey := fmt.Sprintf("system:%d", orgID)
					if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute); err != nil {
						l.Warnw("写入系统统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
					}
				} else {
					l.Warnw("序列化系统统计结果失败", "error", err)
				}
			}

			l.Debugw("从MySQL统计表获取系统统计")
			return stats, nil
		}
	}

	// 3. 最后从原始表实时聚合
	l.Debugw("从原始表实时聚合系统统计")
	result := &statistics.SystemStatistics{
		OrgID:                        orgID,
		AssessmentStatusDistribution: make(map[string]int64),
		AssessmentTrend:              []statistics.DailyCount{},
	}

	// 从 assessments 表聚合测评总数
	var assessmentCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&assessmentCount).Error; err != nil {
		return nil, err
	}
	result.AssessmentCount = assessmentCount

	// 从 testees 表聚合受试者总数
	var testeeCount int64
	if err := s.db.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&testeeCount).Error; err != nil {
		return nil, err
	}
	result.TesteeCount = testeeCount

	// 问卷和答卷数量需要从 MongoDB 查询，暂时设为 0
	// TODO: 如果系统统计服务需要包含问卷和答卷数量，需要注入 MongoDB Repository
	result.QuestionnaireCount = 0
	result.AnswerSheetCount = 0

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

	// 今日新增受试者
	var todayNewTestees int64
	if err := s.db.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND DATE(created_at) = ? AND deleted_at IS NULL", orgID, today).
		Count(&todayNewTestees).Error; err != nil {
		return nil, err
	}
	result.TodayNewTestees = todayNewTestees

	// 今日新增答卷（MongoDB，暂时设为 0）
	result.TodayNewAnswerSheets = 0

	// 近30天趋势（从 statistics_daily 表查询）
	result.AssessmentTrend = s.getDailyTrend(ctx, orgID)

	// 缓存结果（TTL=5分钟）
	if s.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			cacheKey := fmt.Sprintf("system:%d", orgID)
			if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute); err != nil {
				l.Warnw("写入系统统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
			}
		} else {
			l.Warnw("序列化系统统计结果失败", "error", err)
		}
	}

	return result, nil
}

// convertAccumulatedPOToSystemStatistics 转换累计统计PO为系统统计领域对象
func (s *systemStatisticsService) convertAccumulatedPOToSystemStatistics(
	po *statisticsInfra.StatisticsAccumulatedPO,
	orgID int64,
) *statistics.SystemStatistics {
	result := &statistics.SystemStatistics{
		OrgID:                        orgID,
		AssessmentCount:              po.TotalSubmissions,
		AssessmentStatusDistribution: make(map[string]int64),
		AssessmentTrend:              []statistics.DailyCount{},
	}

	// 解析分布数据（状态分布等）
	if po.Distribution != nil {
		if statusDist, ok := po.Distribution["status"].(map[string]interface{}); ok {
			for k, v := range statusDist {
				if count, ok := v.(float64); ok {
					result.AssessmentStatusDistribution[k] = int64(count)
				}
			}
		}
		// 解析其他统计字段
		if questionnaireCount, ok := po.Distribution["questionnaire_count"].(float64); ok {
			result.QuestionnaireCount = int64(questionnaireCount)
		}
		if answerSheetCount, ok := po.Distribution["answer_sheet_count"].(float64); ok {
			result.AnswerSheetCount = int64(answerSheetCount)
		}
		if testeeCount, ok := po.Distribution["testee_count"].(float64); ok {
			result.TesteeCount = int64(testeeCount)
		}
		if todayNewAssessments, ok := po.Distribution["today_new_assessments"].(float64); ok {
			result.TodayNewAssessments = int64(todayNewAssessments)
		}
		if todayNewAnswerSheets, ok := po.Distribution["today_new_answer_sheets"].(float64); ok {
			result.TodayNewAnswerSheets = int64(todayNewAnswerSheets)
		}
		if todayNewTestees, ok := po.Distribution["today_new_testees"].(float64); ok {
			result.TodayNewTestees = int64(todayNewTestees)
		}
	}

	// 趋势数据需要从 statistics_daily 表查询
	result.AssessmentTrend = s.getDailyTrend(context.Background(), orgID)

	return result
}

// getDailyTrend 获取每日趋势数据（近30天）
func (s *systemStatisticsService) getDailyTrend(ctx context.Context, orgID int64) []statistics.DailyCount {
	if s.repo == nil {
		return []statistics.DailyCount{}
	}

	// 查询近30天的每日统计
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	dailyPOs, err := s.repo.GetDailyStatistics(ctx, orgID, statistics.StatisticTypeSystem, "system", startDate, endDate)
	if err != nil || len(dailyPOs) == 0 {
		return []statistics.DailyCount{}
	}

	trend := make([]statistics.DailyCount, 0, len(dailyPOs))
	for _, po := range dailyPOs {
		trend = append(trend, statistics.DailyCount{
			Date:  po.StatDate,
			Count: po.SubmissionCount,
		})
	}

	return trend
}
