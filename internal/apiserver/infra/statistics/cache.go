package statistics

import (
	"context"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

// StatisticsCache 统计查询缓存（Redis 操作封装）。
// 只保留查询结果缓存，不再承担事件去重和日计数写入。
type StatisticsCache struct {
	client redis.UniversalClient
	keys   *rediskey.Builder
	policy cachepolicy.CachePolicy
}

// NewStatisticsCacheWithBuilderAndPolicy 创建绑定显式 key builder/policy 的统计缓存。
func NewStatisticsCacheWithBuilderAndPolicy(client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) *StatisticsCache {
	if builder == nil {
		panic("redis builder is required")
	}
	return &StatisticsCache{
		client: client,
		keys:   builder,
		policy: policy,
	}
}

// GetQueryCache 获取查询结果缓存。
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	start := time.Now()
	result := c.client.Get(ctx, key)
	cacheobservability.ObserveCacheOperationDuration("query_result", "stats_query", "get", time.Since(start))
	if result.Err() == redis.Nil {
		cacheobservability.ObserveCacheGet("query_result", "stats_query", "miss")
		cacheobservability.ObserveFamilySuccess("apiserver", "query_result")
		return "", nil
	}
	if result.Err() != nil {
		cacheobservability.ObserveCacheGet("query_result", "stats_query", "error")
		cacheobservability.ObserveFamilyFailure("apiserver", "query_result", result.Err())
		cacheobservability.ObserveCacheGet("query_result", "stats_query", "miss")
		return "", nil
	}
	data, err := result.Bytes()
	if err != nil {
		cacheobservability.ObserveCacheGet("query_result", "stats_query", "error")
		cacheobservability.ObserveFamilyFailure("apiserver", "query_result", err)
		cacheobservability.ObserveCacheGet("query_result", "stats_query", "miss")
		return "", nil
	}
	cacheobservability.ObserveCacheGet("query_result", "stats_query", "hit")
	cacheobservability.ObserveFamilySuccess("apiserver", "query_result")
	cacheobservability.ObserveCachePayloadBytes("query_result", "stats_query", "stored", len(data))
	raw := c.policy.DecompressValue(data)
	cacheobservability.ObserveCachePayloadBytes("query_result", "stats_query", "raw", len(raw))
	return string(raw), nil
}

// SetQueryCache 设置查询结果缓存。
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	raw := []byte(value)
	payload := c.policy.CompressValue(raw)
	effectiveTTL := c.policy.JitterTTL(c.policy.TTLOr(ttl))
	cacheobservability.ObserveCachePayloadBytes("query_result", "stats_query", "raw", len(raw))
	cacheobservability.ObserveCachePayloadBytes("query_result", "stats_query", "stored", len(payload))
	start := time.Now()
	err := c.client.Set(ctx, key, payload, effectiveTTL).Err()
	cacheobservability.ObserveCacheOperationDuration("query_result", "stats_query", "set", time.Since(start))
	if err != nil {
		cacheobservability.ObserveCacheWrite("query_result", "stats_query", "set", "error")
		cacheobservability.ObserveFamilyFailure("apiserver", "query_result", err)
		return err
	}
	cacheobservability.ObserveFamilySuccess("apiserver", "query_result")
	cacheobservability.ObserveCacheWrite("query_result", "stats_query", "set", "ok")
	return nil
}
