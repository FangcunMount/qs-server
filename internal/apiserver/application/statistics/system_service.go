package statistics

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
)

type systemStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    *statisticsCache.StatisticsCache
	hotset   cachetarget.HotsetRecorder
}

func NewSystemStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache *statisticsCache.StatisticsCache,
	hotset cachetarget.HotsetRecorder,
) SystemStatisticsService {
	return &systemStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
	}
}

func (s *systemStatisticsService) GetSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取系统整体统计", "org_id", orgID)

	cacheKey := fmt.Sprintf("system:%d", orgID)
	if stats, ok := s.loadCachedSystemStatistics(ctx, cacheKey); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		return stats, nil
	}

	if s.query != nil {
		stats, found, err := s.query.LoadSystemStatistics(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if found {
			s.cacheSystemStatistics(ctx, cacheKey, stats)
			l.Debugw("从MySQL统计表获取系统统计")
			s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
			return stats, nil
		}
	}

	l.Debugw("从原始表实时聚合系统统计")
	stats, err := s.realtime.BuildRealtimeSystemStatistics(ctx, orgID)
	if err != nil {
		return nil, err
	}
	s.cacheSystemStatistics(ctx, cacheKey, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
	return stats, nil
}

func (s *systemStatisticsService) loadCachedSystemStatistics(ctx context.Context, cacheKey string) (*statistics.SystemStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	cached, err := s.cache.GetQueryCache(ctx, cacheKey)
	if err != nil || cached == "" {
		return nil, false
	}
	var stats statistics.SystemStatistics
	if err := json.Unmarshal([]byte(cached), &stats); err != nil {
		return nil, false
	}
	logger.L(ctx).Debugw("从Redis缓存获取系统统计", "cache_key", cacheKey)
	return &stats, true
}

func (s *systemStatisticsService) cacheSystemStatistics(ctx context.Context, cacheKey string, stats *statistics.SystemStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	data, err := json.Marshal(stats)
	if err != nil {
		logger.L(ctx).Warnw("序列化系统统计结果失败", "error", err)
		return
	}
	if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 0); err != nil {
		logger.L(ctx).Warnw("写入系统统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
	}
}

func (s *systemStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}
