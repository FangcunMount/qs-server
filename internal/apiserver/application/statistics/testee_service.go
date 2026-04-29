package statistics

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
)

type testeeStatisticsService struct {
	realtime StatisticsRealtimeReader
	cache    *statisticsCache.StatisticsCache
}

func NewTesteeStatisticsService(
	realtime StatisticsRealtimeReader,
	cache *statisticsCache.StatisticsCache,
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

	cacheKey := fmt.Sprintf("testee:%d:%d", orgID, testeeID)
	if stats, ok := s.loadCachedTesteeStatistics(ctx, cacheKey); ok {
		return stats, nil
	}

	l.Debugw("从原始表实时聚合受试者统计")
	stats, err := s.realtime.BuildRealtimeTesteeStatistics(ctx, orgID, testeeID)
	if err != nil {
		return nil, err
	}
	s.cacheTesteeStatistics(ctx, cacheKey, stats)
	return stats, nil
}

func (s *testeeStatisticsService) loadCachedTesteeStatistics(ctx context.Context, cacheKey string) (*statistics.TesteeStatistics, bool) {
	if s.cache == nil {
		return nil, false
	}
	cached, err := s.cache.GetQueryCache(ctx, cacheKey)
	if err != nil || cached == "" {
		return nil, false
	}
	var stats statistics.TesteeStatistics
	if err := json.Unmarshal([]byte(cached), &stats); err != nil {
		return nil, false
	}
	logger.L(ctx).Debugw("从Redis缓存获取受试者统计", "cache_key", cacheKey)
	return &stats, true
}

func (s *testeeStatisticsService) cacheTesteeStatistics(ctx context.Context, cacheKey string, stats *statistics.TesteeStatistics) {
	if s.cache == nil || stats == nil {
		return
	}
	data, err := json.Marshal(stats)
	if err != nil {
		logger.L(ctx).Warnw("序列化受试者统计结果失败", "error", err)
		return
	}
	if err := s.cache.SetQueryCache(ctx, cacheKey, string(data), 0); err != nil {
		logger.L(ctx).Warnw("写入受试者统计查询结果缓存失败", "cache_key", cacheKey, "error", err)
	}
}
