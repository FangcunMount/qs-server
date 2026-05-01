package statistics

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
)

type planStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
	hotset   cachetarget.HotsetRecorder
}

func NewPlanStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
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

	if stats, ok := s.loadCachedPlanStatistics(ctx, orgID, planID); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
		return stats, nil
	}

	if s.query != nil {
		stats, found, err := s.query.LoadPlanStatistics(ctx, orgID, planID)
		if err != nil {
			return nil, err
		}
		if found {
			s.cachePlanStatistics(ctx, orgID, planID, stats)
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
	s.cachePlanStatistics(ctx, orgID, planID, stats)
	s.recordHotset(ctx, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
	return stats, nil
}

func (s *planStatisticsService) loadCachedPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*statistics.PlanStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	return s.cache.LoadPlanStatistics(ctx, orgID, planID)
}

func (s *planStatisticsService) cachePlanStatistics(ctx context.Context, orgID int64, planID uint64, stats *statistics.PlanStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	s.cache.StorePlanStatistics(ctx, orgID, planID, stats)
}

func (s *planStatisticsService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}
