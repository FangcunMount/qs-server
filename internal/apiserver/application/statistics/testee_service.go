package statistics

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticscache "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticscache"
)

type testeeStatisticsService struct {
	realtime StatisticsRealtimeReader
	cache    statisticscache.Cache
}

func NewTesteeStatisticsService(
	realtime StatisticsRealtimeReader,
	cache statisticscache.Cache,
) TesteeStatisticsService {
	return &testeeStatisticsService{
		realtime: realtime,
		cache:    cache,
	}
}

func (s *testeeStatisticsService) GetTesteeStatistics(
	ctx context.Context,
	orgID int64,
	testeeID uint64,
) (*statistics.TesteeStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取受试者统计", "org_id", orgID, "testee_id", testeeID)

	if stats, ok := s.loadCachedTesteeStatistics(ctx, orgID, testeeID); ok {
		return stats, nil
	}

	l.Debugw("从原始表实时聚合受试者统计")
	stats, err := s.realtime.BuildRealtimeTesteeStatistics(ctx, orgID, testeeID)
	if err != nil {
		return nil, err
	}
	s.cacheTesteeStatistics(ctx, orgID, testeeID, stats)
	return stats, nil
}

func (s *testeeStatisticsService) loadCachedTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*statistics.TesteeStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	return s.cache.LoadTesteeStatistics(ctx, orgID, testeeID)
}

func (s *testeeStatisticsService) cacheTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64, stats *statistics.TesteeStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	s.cache.StoreTesteeStatistics(ctx, orgID, testeeID, stats)
}
