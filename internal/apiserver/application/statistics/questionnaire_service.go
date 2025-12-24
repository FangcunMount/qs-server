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

// questionnaireStatisticsService 问卷/量表统计服务实现
type questionnaireStatisticsService struct {
	db         *gorm.DB
	repo       *statisticsInfra.StatisticsRepository
	cache      *statisticsCache.StatisticsCache
	aggregator *statistics.Aggregator
}

// NewQuestionnaireStatisticsService 创建问卷/量表统计服务
func NewQuestionnaireStatisticsService(
	db *gorm.DB,
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
) QuestionnaireStatisticsService {
	return &questionnaireStatisticsService{
		db:         db,
		repo:       repo,
		cache:      cache,
		aggregator: statistics.NewAggregator(),
	}
}

// GetQuestionnaireStatistics 获取问卷/量表统计
func (s *questionnaireStatisticsService) GetQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取问卷统计", "org_id", orgID, "questionnaire_code", questionnaireCode)

	// 1. 优先从Redis缓存查询（查询结果缓存）
	if s.cache != nil {
		cacheKey := fmt.Sprintf("questionnaire:%d:%s", orgID, questionnaireCode)
		cached, err := s.cache.GetQueryCache(ctx, cacheKey)
		if err == nil && cached != "" {
			var stats statistics.QuestionnaireStatistics
			if err := json.Unmarshal([]byte(cached), &stats); err == nil {
				l.Debugw("从Redis缓存获取问卷统计", "cache_key", cacheKey)
				return &stats, nil
			}
		}
	}

	// 2. 其次从MySQL统计表查询
	if s.repo != nil {
		po, err := s.repo.GetAccumulatedStatistics(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode)
		if err == nil && po != nil {
			// 转换为领域对象
			stats := s.convertAccumulatedPOToQuestionnaireStatistics(po, orgID, questionnaireCode)

			// 缓存结果（TTL=5分钟）
			if s.cache != nil {
				if data, err := json.Marshal(stats); err == nil {
					cacheKey := fmt.Sprintf("questionnaire:%d:%s", orgID, questionnaireCode)
					s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute)
				}
			}

			l.Debugw("从MySQL统计表获取问卷统计")
			return stats, nil
		}
	}

	// 3. 最后从原始表实时聚合
	l.Debugw("从原始表实时聚合问卷统计")
	result := &statistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		OriginDistribution: make(map[string]int64),
		DailyTrend:         []statistics.DailyCount{},
	}

	// 从 assessments 表聚合
	var totalSubmissions int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND deleted_at IS NULL", orgID, questionnaireCode).
		Count(&totalSubmissions).Error; err != nil {
		return nil, err
	}
	result.TotalSubmissions = totalSubmissions

	// 总完成数（已解读）
	var totalCompletions int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND status = 'interpreted' AND deleted_at IS NULL",
			orgID, questionnaireCode).
		Count(&totalCompletions).Error; err != nil {
		return nil, err
	}
	result.TotalCompletions = totalCompletions
	result.CompletionRate = s.aggregator.CalculateCompletionRate(totalSubmissions, totalCompletions)

	// 近7/15/30天提交数
	now := time.Now()
	last7d := now.AddDate(0, 0, -7)
	last15d := now.AddDate(0, 0, -15)
	last30d := now.AddDate(0, 0, -30)

	var last7dCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND created_at >= ? AND deleted_at IS NULL",
			orgID, questionnaireCode, last7d).
		Count(&last7dCount).Error; err != nil {
		return nil, err
	}
	result.Last7DaysCount = last7dCount

	var last15dCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND created_at >= ? AND deleted_at IS NULL",
			orgID, questionnaireCode, last15d).
		Count(&last15dCount).Error; err != nil {
		return nil, err
	}
	result.Last15DaysCount = last15dCount

	var last30dCount int64
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND created_at >= ? AND deleted_at IS NULL",
			orgID, questionnaireCode, last30d).
		Count(&last30dCount).Error; err != nil {
		return nil, err
	}
	result.Last30DaysCount = last30dCount

	// 来源分布
	var originCounts []struct {
		OriginType string
		Count      int64
	}
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("origin_type, COUNT(*) as count").
		Where("org_id = ? AND questionnaire_code = ? AND deleted_at IS NULL", orgID, questionnaireCode).
		Group("origin_type").
		Scan(&originCounts).Error; err != nil {
		return nil, err
	}

	for _, oc := range originCounts {
		result.OriginDistribution[oc.OriginType] = oc.Count
	}

	// 趋势数据（从 statistics_daily 表查询）
	result.DailyTrend = s.getDailyTrend(ctx, orgID, questionnaireCode)

	// 缓存结果（TTL=5分钟）
	if s.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			cacheKey := fmt.Sprintf("questionnaire:%d:%s", orgID, questionnaireCode)
			s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute)
		}
	}

	return result, nil
}

// convertAccumulatedPOToQuestionnaireStatistics 转换累计统计PO为问卷统计领域对象
func (s *questionnaireStatisticsService) convertAccumulatedPOToQuestionnaireStatistics(
	po *statisticsInfra.StatisticsAccumulatedPO,
	orgID int64,
	questionnaireCode string,
) *statistics.QuestionnaireStatistics {
	result := &statistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		TotalSubmissions:   po.TotalSubmissions,
		TotalCompletions:   po.TotalCompletions,
		Last7DaysCount:     po.Last7dSubmissions,
		Last15DaysCount:    po.Last15dSubmissions,
		Last30DaysCount:    po.Last30dSubmissions,
		OriginDistribution: make(map[string]int64),
		DailyTrend:         []statistics.DailyCount{},
	}

	result.CompletionRate = s.aggregator.CalculateCompletionRate(po.TotalSubmissions, po.TotalCompletions)

	// 解析分布数据
	if po.Distribution != nil {
		if originDist, ok := po.Distribution["origin"].(map[string]interface{}); ok {
			for k, v := range originDist {
				if count, ok := v.(float64); ok {
					result.OriginDistribution[k] = int64(count)
				}
			}
		}
	}

	return result
}

// getDailyTrend 获取每日趋势数据
func (s *questionnaireStatisticsService) getDailyTrend(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) []statistics.DailyCount {
	if s.repo == nil {
		return []statistics.DailyCount{}
	}

	// 查询近30天的每日统计
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	dailyPOs, err := s.repo.GetDailyStatistics(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode, startDate, endDate)
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
