package statisticscache

import (
	"context"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

const statsQueryCacheKind = "stats:query"

func NewVersionTokenStore(client redis.UniversalClient, health cacheobserve.FamilyObserver) querycache.VersionTokenStore {
	return adapterkit.NewVersionTokenStore(client, cachepolicy.CapabilityStatisticsQuery, health)
}

// StatisticsCache 统计查询缓存（Redis 操作封装）。
// 只保留查询结果缓存，不再承担事件去重和日计数写入。
type StatisticsCache struct {
	cache        sharedcache.Store
	versionStore querycache.VersionTokenStore
	policies     sharedcache.PolicyProvider
	observer     *observability.ComponentObserver
	keys         *keyspace.Builder
	coalescer    loadguard.Coalescer
}

// NewStatisticsCacheWithBuilderAndPolicy 创建绑定显式 key builder/policy 的统计缓存。
func NewStatisticsCacheWithBuilderAndProvider(client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider) *StatisticsCache {
	return NewStatisticsCacheWithBuilderProviderVersionStoreAndObserver(
		client,
		builder,
		policies,
		querycache.NewStaticVersionTokenStore(0),
		nil,
	)
}

func NewStatisticsCacheWithBuilderProviderVersionStoreAndObserver(
	client redis.UniversalClient,
	builder *keyspace.Builder,
	policies sharedcache.PolicyProvider,
	versionStore querycache.VersionTokenStore,
	observer *observability.ComponentObserver,
) *StatisticsCache {
	if builder == nil {
		panic("redis builder is required")
	}
	if versionStore == nil {
		versionStore = querycache.NewStaticVersionTokenStore(0)
	}
	return &StatisticsCache{
		cache:        redisstore.NewStore(client),
		versionStore: versionStore,
		policies:     policies,
		observer:     observer,
		keys:         builder,
		coalescer:    loadguard.NewCoalescer(true),
	}
}

// GetQueryCache 获取查询结果缓存。
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	if c == nil || c.cache == nil {
		return "", nil
	}
	var value string
	err := c.queryCache().Get(ctx, c.versionKey(cacheKey), func(version uint64) string {
		return c.dataKey(cacheKey, version)
	}, &value)
	if err == sharedcache.ErrMiss {
		return "", nil
	}
	if err != nil {
		return "", nil
	}
	return value, nil
}

// SetQueryCache 设置查询结果缓存。
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string) error {
	if c == nil || c.cache == nil {
		return nil
	}
	c.queryCache().Set(ctx, c.versionKey(cacheKey), func(version uint64) string {
		return c.dataKey(cacheKey, version)
	}, value)
	return nil
}

func (c *StatisticsCache) queryCache() *querycache.Versioned {
	if c == nil || c.cache == nil || c.versionStore == nil {
		return nil
	}
	return querycache.NewVersioned(querycache.VersionedOptions{
		Store:      c.cache,
		Version:    c.versionStore,
		Capability: sharedcache.Capability(cachepolicy.CapabilityStatisticsQuery),
		Policies:   c.policies,
		Observer: cacheobserve.NewPrometheus(
			string(cachepolicy.Family(cachepolicy.CapabilityStatisticsQuery)),
			cachepolicy.MetricLabel(cachepolicy.CapabilityStatisticsQuery),
			c.observer,
		),
	})
}

func (c *StatisticsCache) versionKey(cacheKey string) string {
	return c.keys.BuildQueryVersionKey(statsQueryCacheKind, cacheKey)
}

func (c *StatisticsCache) dataKey(cacheKey string, version uint64) string {
	return c.keys.BuildVersionedQueryKey(statsQueryCacheKind, cacheKey, version, "")
}
