package redislock

import (
	"context"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`)

// Acquire 获取分布式锁，返回 token、是否获取成功以及错误
func Acquire(ctx context.Context, client redis.UniversalClient, key string, ttl time.Duration) (string, bool, error) {
	if client == nil {
		return "", false, nil
	}

	token := uuid.NewString()
	ok, err := client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

// Release 释放分布式锁（只有 token 匹配时才删除）
func Release(ctx context.Context, client redis.UniversalClient, key, token string) error {
	if client == nil || token == "" {
		return nil
	}
	_, err := releaseScript.Run(ctx, client, []string{key}, token).Result()
	if err == redis.Nil {
		return nil
	}
	return err
}
