package redisstore

import (
	"context"
	"fmt"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redis "github.com/redis/go-redis/v9"
)

// RedisCache 实现最小 Cache 接口，提供统一的 Redis 缓存读写操作。
type Store struct {
	client redis.UniversalClient
}

func NewStore(client redis.UniversalClient) sharedcache.Store {
	return &Store{client: client}
}

func (c *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return nil, sharedcache.ErrMiss
	}
	if result.Err() != nil {
		return nil, result.Err()
	}

	return []byte(result.Val()), nil
}

func (c *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *Store) Delete(ctx context.Context, key string) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return c.client.Del(ctx, key).Err()
}

func (c *Store) Exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("redis client is nil")
	}
	result := c.client.Exists(ctx, key)
	if result.Err() != nil {
		return false, result.Err()
	}
	return result.Val() > 0, nil
}
