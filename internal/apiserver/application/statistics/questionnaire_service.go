package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
)

type questionnaireStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    *statisticsCache.StatisticsCache
	hotset   cachetarget.HotsetRecorder
}

func NewQuestionnaireStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache *statisticsCache.StatisticsCache,
	hotset cachetarget.HotsetRecorder,
) QuestionnaireStatisticsService {
	return &questionnaireStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
	}
}

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

	if s.query != nil {
		stats, found, err := s.query.LoadQuestionnaireStatistics(ctx, orgID, questionnaireCode)
		if err != nil {
			return nil, err
		}
		if found {
			s.cacheQuestionnaireStatistics(ctx, cacheKey, stats)
			l.Debugw("从MySQL统计表获取问卷统计")
			s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
			return stats, nil
		}
	}

	l.Debugw("从原始表实时聚合问卷统计")
	stats, err := s.realtime.BuildRealtimeQuestionnaireStatistics(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, err
	}
	s.cacheQuestionnaireStatistics(ctx, cacheKey, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, questionnaireCode))
	return stats, nil
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

func (s *questionnaireStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}
