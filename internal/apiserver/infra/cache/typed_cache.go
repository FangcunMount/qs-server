package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TypedRedisCache 类型化 Redis 缓存实现
// 提供类型安全的缓存操作，自动处理序列化/反序列化
type TypedRedisCache[T any] struct {
	cache Cache
}

// NewTypedCache 创建类型化缓存实例
func NewTypedCache[T any](cache Cache) TypedCache[T] {
	return &TypedRedisCache[T]{
		cache: cache,
	}
}

// Get 获取缓存值
func (c *TypedRedisCache[T]) Get(ctx context.Context, key string) (*T, error) {
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		if err == ErrCacheNotFound {
			return nil, ErrCacheNotFound
		}
		return nil, fmt.Errorf("failed to get cache: %w", err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return &value, nil
}

// Set 设置缓存值
func (c *TypedRedisCache[T]) Set(ctx context.Context, key string, value *T, ttl time.Duration) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	return c.cache.Set(ctx, key, data, ttl)
}

// Delete 删除缓存
func (c *TypedRedisCache[T]) Delete(ctx context.Context, key string) error {
	return c.cache.Delete(ctx, key)
}

// Exists 检查键是否存在
func (c *TypedRedisCache[T]) Exists(ctx context.Context, key string) (bool, error) {
	return c.cache.Exists(ctx, key)
}
