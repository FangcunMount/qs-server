package statistics

import (
	"context"
	"fmt"
	"sync"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"golang.org/x/sync/singleflight"
)

type systemStatisticsService struct {
	query    StatisticsQueryReader
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
	hotset   cachetarget.HotsetRecorder
	opts     SystemStatisticsOptions

	sfGroup singleflight.Group
	stale   sync.Map // orgID int64 -> *statistics.SystemStatistics
}

type SystemStatisticsServiceOption func(*systemStatisticsService)

func WithSystemStatisticsOptions(opts SystemStatisticsOptions) SystemStatisticsServiceOption {
	return func(s *systemStatisticsService) {
		s.opts = opts
	}
}

func NewSystemStatisticsService(
	query StatisticsQueryReader,
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
	hotset cachetarget.HotsetRecorder,
	opts ...SystemStatisticsServiceOption,
) SystemStatisticsService {
	service := &systemStatisticsService{
		query:    query,
		realtime: realtime,
		cache:    cache,
		hotset:   hotset,
		opts:     DefaultSystemStatisticsOptions(),
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
		s.rememberStale(orgID, stats)
		return stats, nil
	}

	if !s.opts.ServiceSingleflight {
		stats, err := s.loadSystemStatisticsMiss(ctx, orgID)
		if err != nil {
			return nil, err
		}
		s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		return stats, nil
	}

	key := fmt.Sprintf("system-stats:%d", orgID)
	value, err, _ := s.sfGroup.Do(key, func() (interface{}, error) {
		if stats, ok := s.loadCachedSystemStatistics(ctx, orgID); ok {
			return stats, nil
		}
		return s.loadSystemStatisticsMiss(ctx, orgID)
	})
	if err != nil {
		return nil, err
	}
	stats, ok := value.(*statistics.SystemStatistics)
	if !ok || stats == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "invalid system statistics payload")
	}
	s.recordHotset(ctx, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
	return stats, nil
}

func (s *systemStatisticsService) loadSystemStatisticsMiss(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	loader := func(loadCtx context.Context) (*statistics.SystemStatistics, error) {
		return s.computeSystemStatistics(loadCtx, orgID)
	}
	if coalescer, ok := s.cache.(statisticscache.SystemStatisticsLoader); ok && s.cache != nil {
		stats, err := coalescer.LoadSystemStatisticsCoalesced(ctx, orgID, loader)
		if err != nil {
			if stale, ok := s.tryStaleOnError(orgID, err); ok {
				logger.L(ctx).Warnw("系统统计回源失败，返回进程内陈旧缓存", "org_id", orgID, "error", err)
				return stale, nil
			}
			return nil, err
		}
		if stats != nil {
			s.rememberStale(orgID, stats)
		}
		return stats, nil
	}

	stats, err := loader(ctx)
	if err != nil {
		if stale, ok := s.tryStaleOnError(orgID, err); ok {
			logger.L(ctx).Warnw("系统统计回源失败，返回进程内陈旧缓存", "org_id", orgID, "error", err)
			return stale, nil
		}
		return nil, err
	}
	s.cacheSystemStatistics(ctx, orgID, stats)
	if stats != nil {
		s.rememberStale(orgID, stats)
	}
	return stats, nil
}

func (s *systemStatisticsService) computeSystemStatistics(ctx context.Context, orgID int64) (*statistics.SystemStatistics, error) {
	loadCtx := ctx
	if s.opts.LoadTimeout > 0 {
		var cancel context.CancelFunc
		loadCtx, cancel = context.WithTimeout(ctx, s.opts.LoadTimeout)
		defer cancel()
	}

	if s.query != nil {
		stats, found, err := s.query.LoadSystemStatistics(loadCtx, orgID)
		if err != nil {
			return nil, err
		}
		if found {
			logger.L(loadCtx).Debugw("从MySQL统计表获取系统统计")
			return stats, nil
		}
	}

	if s.opts.DisableRealtimeFallback {
		if stale, ok := s.loadStale(orgID); ok {
			logger.L(loadCtx).Warnw("系统统计快照未就绪，返回进程内陈旧缓存", "org_id", orgID)
			return stale, nil
		}
		return nil, errors.WithCode(code.ErrInternalServerError, "system statistics snapshot is not ready")
	}

	logger.L(loadCtx).Debugw("从原始表实时聚合系统统计")
	return s.realtime.BuildRealtimeSystemStatistics(loadCtx, orgID)
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

func (s *systemStatisticsService) rememberStale(orgID int64, stats *statistics.SystemStatistics) {
	if stats == nil {
		return
	}
	s.stale.Store(orgID, cloneSystemStatistics(stats))
}

func (s *systemStatisticsService) loadStale(orgID int64) (*statistics.SystemStatistics, bool) {
	if !s.opts.StaleOnTimeout {
		return nil, false
	}
	value, ok := s.stale.Load(orgID)
	if !ok {
		return nil, false
	}
	stats, ok := value.(*statistics.SystemStatistics)
	if !ok || stats == nil {
		return nil, false
	}
	return cloneSystemStatistics(stats), true
}

func (s *systemStatisticsService) tryStaleOnError(orgID int64, err error) (*statistics.SystemStatistics, bool) {
	if err == nil || !s.opts.StaleOnTimeout {
		return nil, false
	}
	return s.loadStale(orgID)
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
