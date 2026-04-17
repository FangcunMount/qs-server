package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

// StatisticsCache 统计查询缓存（Redis 操作封装）。
// 只保留查询结果缓存，不再承担事件去重和日计数写入。
type StatisticsCache struct {
	client redis.UniversalClient
	keys   *rediskey.Builder
}

// NewStatisticsCache 创建统计缓存。
func NewStatisticsCache(client redis.UniversalClient) *StatisticsCache {
	return &StatisticsCache{
		client: client,
		keys:   rediskey.NewBuilder(),
	}
}

// GetQueryCache 获取查询结果缓存。
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return "", nil
	}
	return result.Val(), result.Err()
}

// SetQueryCache 设置查询结果缓存。
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	return c.client.Set(ctx, key, value, ttl).Err()
}
