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

type planStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    *statisticsCache.StatisticsCache
	hotset   cachetarget.HotsetRecorder
}

func NewPlanStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache *statisticsCache.StatisticsCache,
	hotset cachetarget.HotsetRecorder,
) PlanStatisticsService {
	return &planStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
	}
}

func (s *planStatisticsService) GetPlanStatistics(
	ctx context.Context,
	orgID int64,
	planID uint64,
) (*statistics.PlanStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取计划统计", "org_id", orgID, "plan_id", planID)

	cacheKey := planStatsCacheKey(orgID, planID)
	if stats, ok := s.loadCachedPlanStatistics(ctx, cacheKey); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
		return stats, nil
	}

	if s.query != nil {
		stats, found, err := s.query.LoadPlanStatistics(ctx, orgID, planID)
		if err != nil {
			return nil, err
		}
		if found {
			s.cachePlanStatistics(ctx, cacheKey, stats)
			l.Debugw("从MySQL统计表获取计划统计")
			s.recordHotset(ctx, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
			return stats, nil
		}
	}

	l.Debugw("从原始表实时聚合计划统计")
	stats, err := s.realtime.BuildRealtimePlanStatistics(ctx, orgID, planID)
	if err != nil {
		return nil, err
	}
	s.cachePlanStatistics(ctx, cacheKey, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
	return stats, nil
}

func planStatsCacheKey(orgID int64, planID uint64) string {
	return fmt.Sprintf("plan:%d:%d", orgID, planID)
}

func (s *planStatisticsService) loadCachedPlanStatistics(ctx context.Context, cacheKey string) (*statistics.PlanStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	cached, err := s.cache.GetQueryCache(ctx, cacheKey)
	if err != nil || cached == "" {
		return nil, false
	}
	var stats statistics.PlanStatistics
	if err := json.Unmarshal([]byte(cached), &stats); err != nil {
		return nil, false
	}
	logger.L(ctx).Debugw("从Redis缓存获取计划统计", "cache_key", cacheKey)
	return &stats, true
}

func (s *planStatisticsService) cachePlanStatistics(ctx context.Context, cacheKey string, stats *statistics.PlanStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	data, err := json.Marshal(stats)
	if err != nil {
		logger.L(ctx).Warnw("序列化计划统计结果失败", "error", err)
		return
	}
	if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 0); err != nil {
		logger.L(ctx).Warnw("写入计划统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
	}
}

func (s *planStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}
