package wechatapi

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2/cache"
)

// RedisCacheAdapter 将 Redis 客户端适配为微信 SDK 的 cache.Cache 接口
// 用于微信 SDK 的 access_token 缓存
type RedisCacheAdapter struct {
	client redis.UniversalClient
	prefix string // 缓存键前缀，避免与其他缓存冲突
}

// NewRedisCacheAdapter 创建 Redis 缓存适配器
func NewRedisCacheAdapter(client redis.UniversalClient) cache.Cache {
	if client == nil {
		// 如果 Redis 客户端为 nil，返回内存缓存
		return cache.NewMemory()
	}
	return &RedisCacheAdapter{
		client: client,
		prefix: "wechat:cache:", // 微信 SDK 缓存键前缀
	}
}

// buildKey 构建完整的缓存键
func (a *RedisCacheAdapter) buildKey(key string) string {
	return a.prefix + key
}

// Get 获取缓存值
func (a *RedisCacheAdapter) Get(key string) interface{} {
	if a.client == nil {
		return nil
	}

	ctx := context.Background()
	fullKey := a.buildKey(key)
	val, err := a.client.Get(ctx, fullKey).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		// 获取失败时返回 nil，不阻塞流程
		return nil
	}
	return val
}

// Set 设置缓存值
func (a *RedisCacheAdapter) Set(key string, val interface{}, timeout time.Duration) error {
	if a.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()
	fullKey := a.buildKey(key)

	// 将值转换为字符串（微信 SDK 的 access_token 是字符串）
	var strVal string
	switch v := val.(type) {
	case string:
		strVal = v
	case []byte:
		strVal = string(v)
	default:
		strVal = fmt.Sprintf("%v", v)
	}

	return a.client.Set(ctx, fullKey, strVal, timeout).Err()
}

// IsExist 检查键是否存在
func (a *RedisCacheAdapter) IsExist(key string) bool {
	if a.client == nil {
		return false
	}

	ctx := context.Background()
	fullKey := a.buildKey(key)
	result := a.client.Exists(ctx, fullKey)
	if result.Err() != nil {
		return false
	}
	return result.Val() > 0
}

// Delete 删除缓存
func (a *RedisCacheAdapter) Delete(key string) error {
	if a.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()
	fullKey := a.buildKey(key)
	return a.client.Del(ctx, fullKey).Err()
}
