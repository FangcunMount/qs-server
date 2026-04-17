package redislock

import (
	"context"
	"time"

	rediskit "github.com/FangcunMount/qs-server/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Acquire 获取分布式锁，返回 token、是否获取成功以及错误。
func Acquire(ctx context.Context, client goredis.UniversalClient, key string, ttl time.Duration) (string, bool, error) {
	if client == nil {
		return "", false, nil
	}
	return rediskit.AcquireLease(ctx, client, key, ttl)
}

// Release 释放分布式锁，仅当 token 匹配时才删除。
func Release(ctx context.Context, client goredis.UniversalClient, key, token string) error {
	if client == nil || token == "" {
		return nil
	}
	return rediskit.ReleaseLease(ctx, client, key, token)
}
