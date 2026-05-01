package statistics

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
)

type systemStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
	hotset   cachetarget.HotsetRecorder
}

func NewSystemStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
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

	if stats, ok := s.loadCachedSystemStatistics(ctx, orgID); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		return stats, nil
	}

	if s.query != nil {
		stats, found, err := s.query.LoadSystemStatistics(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if found {
			s.cacheSystemStatistics(ctx, orgID, stats)
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
	s.cacheSystemStatistics(ctx, orgID, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
	return stats, nil
}

func (s *systemStatisticsService) loadCachedSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	return s.cache.LoadSystemStatistics(ctx, orgID)
}

func (s *systemStatisticsService) cacheSystemStatistics(ctx context.Context, orgID int64, stats *statistics.SystemStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	s.cache.StoreSystemStatistics(ctx, orgID, stats)
}

func (s *systemStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}
