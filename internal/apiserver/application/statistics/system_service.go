package statistics

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type systemStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
	hotset   cachetarget.HotsetRecorder
	opts     SystemStatisticsOptions
	guard    *loadguard.Guard[int64, *statistics.SystemStatistics]
}

type SystemStatisticsServiceOption func(*systemStatisticsService)

func WithSystemStatisticsOptions(opts SystemStatisticsOptions) SystemStatisticsServiceOption {
	return func(s *systemStatisticsService) {
		s.opts = opts
		s.guard = loadguard.New[int64, *statistics.SystemStatistics](
			opts.ToLoadGuardPolicy(),
			cloneSystemStatistics,
			nil,
		)
	}
}

func NewSystemStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
	hotset cachetarget.HotsetRecorder,
	opts ...SystemStatisticsServiceOption,
) SystemStatisticsService {
	defaultOpts := DefaultSystemStatisticsOptions()
	service := &systemStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
		opts:     defaultOpts,
		guard: loadguard.New[int64, *statistics.SystemStatistics](
			defaultOpts.ToLoadGuardPolicy(),
			cloneSystemStatistics,
			nil,
		),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *systemStatisticsService) GetSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取系统整体统计", "org_id", orgID)

	if stats, ok := s.loadCachedSystemStatistics(ctx, orgID); ok {
		s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		s.guard.RememberStale(orgID, stats)
		return stats, nil
	}

	stats, err := s.guard.Load(ctx, orgID, func(loadCtx context.Context) (*statistics.SystemStatistics, error) {
		if cached, ok := s.loadCachedSystemStatistics(loadCtx, orgID); ok {
			return cached, nil
		}
		return s.loadSystemStatisticsMiss(loadCtx, orgID)
	})
	if err != nil {
		return nil, err
	}
	s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
	return stats, nil
}

func (s *systemStatisticsService) loadSystemStatisticsMiss(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	loader := func(loadCtx context.Context) (*statistics.SystemStatistics, error) {
		return s.computeSystemStatistics(loadCtx, orgID)
	}
	if coalescer, ok := s.cache.(statisticscache.SystemStatisticsLoader); ok && s.cache != nil {
		return coalescer.LoadSystemStatisticsCoalesced(ctx, orgID, loader)
	}

	stats, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	s.cacheSystemStatistics(ctx, orgID, stats)
	return stats, nil
}

func (s *systemStatisticsService) computeSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	if s.query != nil {
		stats, found, err := s.query.LoadSystemStatistics(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if found {
			logger.L(ctx).Debugw("从MySQL统计表获取系统统计")
			return stats, nil
		}
	}

	if s.opts.DisableRealtimeFallback {
		if stale, ok := s.guard.LoadStale(orgID); ok {
			logger.L(ctx).Warnw("系统统计快照未就绪，返回进程内陈旧缓存", "org_id", orgID)
			return stale, nil
		}
		return nil, errors.WithCode(code.ErrInternalServerError, "system statistics snapshot is not ready")
	}

	logger.L(ctx).Debugw("从原始表实时聚合系统统计")
	return s.realtime.BuildRealtimeSystemStatistics(ctx, orgID)
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

func cloneSystemStatistics(stats *statistics.SystemStatistics) *statistics.SystemStatistics {
	if stats == nil {
		return nil
	}
	cloned := *stats
	if stats.AssessmentStatusDistribution != nil {
		cloned.AssessmentStatusDistribution = make(map[string]int64, len(stats.AssessmentStatusDistribution))
		for key, value := range stats.AssessmentStatusDistribution {
			cloned.AssessmentStatusDistribution[key] = value
		}
	}
	if len(stats.AssessmentTrend) > 0 {
		cloned.AssessmentTrend = append([]statistics.DailyCount(nil), stats.AssessmentTrend...)
	}
	return &cloned
}
