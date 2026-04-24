package cache

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	redis "github.com/redis/go-redis/v9"
)

type RedisCache = cacheentry.RedisCache

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(client redis.UniversalClient) Cache {
	return cacheentry.NewRedisCache(client)
}

func newRedisCacheIfAvailable(client redis.UniversalClient) Cache {
	if client == nil {
		return nil
	}
	return NewRedisCache(client)
}
