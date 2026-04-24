package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const statsQueryCacheKind = "stats:query"

// StatisticsCache 统计查询缓存（Redis 操作封装）。
// 只保留查询结果缓存，不再承担事件去重和日计数写入。
type StatisticsCache struct {
	cache        cacheentry.Cache
	versionStore cachequery.VersionTokenStore
	policy       cachepolicy.CachePolicy
	observer     *cacheobservability.ComponentObserver
	keys         *rediskey.Builder
}

// NewStatisticsCacheWithBuilderAndPolicy 创建绑定显式 key builder/policy 的统计缓存。
func NewStatisticsCacheWithBuilderAndPolicy(client redis.UniversalClient, builder *rediskey.Builder, policy cachepolicy.CachePolicy) *StatisticsCache {
	return NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
		client,
		builder,
		policy,
		cachequery.NewStaticVersionTokenStore(0),
		nil,
	)
}

func NewStatisticsCacheWithBuilderPolicyVersionStoreAndObserver(
	client redis.UniversalClient,
	builder *rediskey.Builder,
	policy cachepolicy.CachePolicy,
	versionStore cachequery.VersionTokenStore,
	observer *cacheobservability.ComponentObserver,
) *StatisticsCache {
	if builder == nil {
		panic("redis builder is required")
	}
	if versionStore == nil {
		versionStore = cachequery.NewStaticVersionTokenStore(0)
	}
	return &StatisticsCache{
		cache:        cacheentry.NewRedisCache(client),
		versionStore: versionStore,
		policy:       policy,
		observer:     observer,
		keys:         builder,
	}
}

// GetQueryCache 获取查询结果缓存。
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	if c == nil || c.cache == nil {
		return "", nil
	}
	var value string
	err := c.queryCache(0).Get(ctx, c.versionKey(cacheKey), func(version uint64) string {
		return c.dataKey(cacheKey, version)
	}, &value)
	if err == cacheentry.ErrCacheNotFound {
		return "", nil
	}
	if err != nil {
		return "", nil
	}
	return value, nil
}

// SetQueryCache 设置查询结果缓存。
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	if c == nil || c.cache == nil {
		return nil
	}
	c.queryCache(ttl).Set(ctx, c.versionKey(cacheKey), func(version uint64) string {
		return c.dataKey(cacheKey, version)
	}, value)
	return nil
}

func (c *StatisticsCache) queryCache(ttl time.Duration) *cachequery.VersionedQueryCache {
	if c == nil || c.cache == nil || c.versionStore == nil {
		return nil
	}
	return cachequery.NewVersionedQueryCacheWithObserver(
		c.cache,
		c.versionStore,
		cachepolicy.PolicyStatsQuery,
		c.policy,
		ttl,
		nil,
		c.observer,
	)
}

func (c *StatisticsCache) versionKey(cacheKey string) string {
	return c.keys.BuildQueryVersionKey(statsQueryCacheKind, cacheKey)
}

func (c *StatisticsCache) dataKey(cacheKey string, version uint64) string {
	return c.keys.BuildVersionedQueryKey(statsQueryCacheKind, cacheKey, version, "")
}
