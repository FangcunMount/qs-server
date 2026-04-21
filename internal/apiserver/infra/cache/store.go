package cache

import (
	"context"
	"time"
)

// Cache 表示 apiserver 缓存实现层需要的最小 Redis 存储接口。
// 它只保留主路径真实使用的能力，不再承载批量操作、模式删除或健康检查。
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}
