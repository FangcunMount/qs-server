package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
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
	hotset     cachetarget.HotsetRecorder
}

// NewQuestionnaireStatisticsService 创建问卷/量表统计服务
func NewQuestionnaireStatisticsService(
	db *gorm.DB,
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
	hotset cachetarget.HotsetRecorder,
) QuestionnaireStatisticsService {
	return &questionnaireStatisticsService{
		db:         db,
		repo:       repo,
		cache:      cache,
		aggregator: statistics.NewAggregator(),
		hotset:     hotset,
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
	cacheKey := questionnaireStatsCacheKey(orgID, questionnaireCode)

	if stats, ok := s.loadCachedQuestionnaireStatistics(ctx, cacheKey); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
		return stats, nil
	}

	stats, found, err := s.loadAccumulatedQuestionnaireStatistics(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, err
	}
	if found {
		s.cacheQuestionnaireStatistics(ctx, cacheKey, stats)
		l.Debugw("从MySQL统计表获取问卷统计")
		s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
		return stats, nil
	}

	l.Debugw("从原始表实时聚合问卷统计")
	realtimeStats, err := s.buildRealtimeQuestionnaireStatistics(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, err
	}

	s.cacheQuestionnaireStatistics(ctx, cacheKey, realtimeStats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
	return realtimeStats, nil
}

func questionnaireStatsCacheKey(orgID int64, questionnaireCode string) string {
	return fmt.Sprintf("questionnaire:%d:%s", orgID, questionnaireCode)
}

func (s *questionnaireStatisticsService) loadCachedQuestionnaireStatistics(
	ctx context.Context,
	cacheKey string,
) (*statistics.QuestionnaireStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}

	cached, err := s.cache.GetQueryCache(ctx, cacheKey)
	if err != nil || cached == "" {
		return nil, false
	}

	var stats statistics.QuestionnaireStatistics
	if err := json.Unmarshal([]byte(cached), &stats); err != nil {
		return nil, false
	}

	logger.L(ctx).Debugw("从Redis缓存获取问卷统计", "cache_key", cacheKey)
	return &stats, true
}

func (s *questionnaireStatisticsService) loadAccumulatedQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, bool, error) {
	if s.repo == nil {
		return nil, false, nil
	}

	po, err := s.repo.GetAccumulatedStatistics(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode)
	if err != nil || po == nil {
		return nil, false, err
	}

	stats := s.convertAccumulatedPOToQuestionnaireStatistics(po, orgID, questionnaireCode)
	stats.DailyTrend = s.getDailyTrend(ctx, orgID, questionnaireCode)
	if len(stats.OriginDistribution) == 0 {
		originDistribution, originErr := s.getOriginDistribution(ctx, orgID, questionnaireCode)
		if originErr != nil {
			logger.L(ctx).Warnw("查询问卷来源分布失败", "questionnaire_code", questionnaireCode, "error", originErr)
		} else {
			stats.OriginDistribution = originDistribution
		}
	}

	return stats, true, nil
}

func (s *questionnaireStatisticsService) buildRealtimeQuestionnaireStatistics(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (*statistics.QuestionnaireStatistics, error) {
	result := &statistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		OriginDistribution: make(map[string]int64),
		DailyTrend:         []statistics.DailyCount{},
	}

	totalSubmissions, err := s.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, nil, "")
	if err != nil {
		return nil, err
	}
	totalCompletions, err := s.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, nil, "interpreted")
	if err != nil {
		return nil, err
	}
	result.TotalSubmissions = totalSubmissions
	result.TotalCompletions = totalCompletions
	result.CompletionRate = s.aggregator.CalculateCompletionRate(totalSubmissions, totalCompletions)

	last7dCount, err := s.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(7), "")
	if err != nil {
		return nil, err
	}
	last15dCount, err := s.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(15), "")
	if err != nil {
		return nil, err
	}
	last30dCount, err := s.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(30), "")
	if err != nil {
		return nil, err
	}
	result.Last7DaysCount = last7dCount
	result.Last15DaysCount = last15dCount
	result.Last30DaysCount = last30dCount

	originDistribution, err := s.getOriginDistribution(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, err
	}
	result.OriginDistribution = originDistribution
	result.DailyTrend = s.getDailyTrend(ctx, orgID, questionnaireCode)

	return result, nil
}

func (s *questionnaireStatisticsService) countQuestionnaireAssessments(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
	createdAfter *time.Time,
	status string,
) (int64, error) {
	query := s.db.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND deleted_at IS NULL", orgID, questionnaireCode)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if createdAfter != nil {
		query = query.Where("created_at >= ?", *createdAfter)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *questionnaireStatisticsService) cacheQuestionnaireStatistics(
	ctx context.Context,
	cacheKey string,
	stats *statistics.QuestionnaireStatistics,
) {
	if s.cache == nil || stats == nil {
		return
	}

	data, err := json.Marshal(stats)
	if err != nil {
		logger.L(ctx).Warnw("序列化问卷统计结果失败", "error", err)
		return
	}
	if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 0); err != nil {
		logger.L(ctx).Warnw("写入问卷统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
	}
}

func daysAgo(days int) *time.Time {
	t := time.Now().AddDate(0, 0, -days)
	return &t
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

func (s *questionnaireStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func (s *questionnaireStatisticsService) getOriginDistribution(
	ctx context.Context,
	orgID int64,
	questionnaireCode string,
) (map[string]int64, error) {
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

	distribution := make(map[string]int64)
	for _, oc := range originCounts {
		distribution[oc.OriginType] = oc.Count
	}

	return distribution, nil
}
