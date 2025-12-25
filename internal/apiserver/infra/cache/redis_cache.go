package cache

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisCache Redis 缓存实现
// 实现 Cache 接口，提供统一的缓存操作
type RedisCache struct {
	client redis.UniversalClient
}

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(client redis.UniversalClient) Cache {
	return &RedisCache{
		client: client,
	}
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

// MGet 批量获取
func (c *RedisCache) MGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	// Redis MGet 接受 []string
	result := c.client.MGet(ctx, keys...)
	if result.Err() != nil {
		return nil, result.Err()
	}

	values := result.Val()
	resultMap := make(map[string][]byte, len(keys))
	for i, val := range values {
		if val != nil {
			if str, ok := val.(string); ok {
				resultMap[keys[i]] = []byte(str)
			}
		}
	}

	return resultMap, nil
}

// MSet 批量设置
func (c *RedisCache) MSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	if len(items) == 0 {
		return nil
	}

	// 使用 Pipeline 批量设置
	pipe := c.client.Pipeline()
	for key, value := range items {
		pipe.Set(ctx, key, value, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// DeletePattern 按模式删除（谨慎使用，性能较低）
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	var cursor uint64
	var deletedCount int

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			deletedCount += len(keys)
		}

		if cursor == 0 {
			break
		}
	}

	return nil
}

// Ping 健康检查
func (c *RedisCache) Ping(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return c.client.Ping(ctx).Err()
}

// ErrCacheNotFound 缓存未找到错误
var ErrCacheNotFound = fmt.Errorf("cache not found")

