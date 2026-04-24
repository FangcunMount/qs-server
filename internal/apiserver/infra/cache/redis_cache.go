package cache

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisCache Redis 缓存实现
// 实现最小 Cache 接口，提供统一的缓存读写操作。
type RedisCache struct {
	client redis.UniversalClient
}

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(client redis.UniversalClient) Cache {
	return &RedisCache{
		client: client,
	}
}

func newRedisCacheIfAvailable(client redis.UniversalClient) Cache {
	if client == nil {
		return nil
	}
	return NewRedisCache(client)
}

// Get 获取缓存值
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return nil, ErrCacheNotFound
	}
	if result.Err() != nil {
		return nil, result.Err()
	}

	return []byte(result.Val()), nil
}

// Set 设置缓存值
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete 删除缓存
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return c.client.Del(ctx, key).Err()
}

// Exists 检查键是否存在
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}

	result := c.client.Exists(ctx, key)
	if result.Err() != nil {
		return false, result.Err()
	}

	return result.Val() > 0, nil
}

// ErrCacheNotFound 缓存未找到错误
var ErrCacheNotFound = fmt.Errorf("cache not found")
