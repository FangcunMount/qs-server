package wechatapi

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	redis "github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2/cache"
)

// RedisCacheAdapter 将 Redis 客户端适配为微信 SDK 的 cache.Cache 接口
// 用于微信 SDK 的 access_token 缓存
type RedisCacheAdapter struct {
	client   redis.UniversalClient
	keys     *keyspace.Builder
	observer *observability.ComponentObserver
}

// NewRedisCacheAdapterWithBuilder 创建带显式 key builder 的 Redis 缓存适配器。
func NewRedisCacheAdapterWithBuilder(client redis.UniversalClient, builder *keyspace.Builder) cache.Cache {
	return NewRedisCacheAdapterWithBuilderAndObserver(client, builder, observability.NewComponentObserver("apiserver"))
}

// NewRedisCacheAdapterWithBuilderAndObserver 创建带显式 observer 的 Redis 缓存适配器。
func NewRedisCacheAdapterWithBuilderAndObserver(client redis.UniversalClient, builder *keyspace.Builder, observer *observability.ComponentObserver) cache.Cache {
	if client == nil {
		// 如果 Redis 客户端为 nil，返回内存缓存
		return cache.NewMemory()
	}
	if builder == nil {
		panic("redis builder is required")
	}
	return &RedisCacheAdapter{
		client:   client,
		keys:     builder,
		observer: observer,
	}
}

// buildKey 构建完整的缓存键
func (a *RedisCacheAdapter) buildKey(key string) string {
	return a.keys.BuildWeChatCacheKey(key)
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
		a.observeSuccess()
		return nil
	}
	if err != nil {
		a.observeFailure(err)
		return nil
	}
	a.observeSuccess()
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

	if err := a.client.Set(ctx, fullKey, strVal, timeout).Err(); err != nil {
		a.observeFailure(err)
		return nil
	}
	a.observeSuccess()
	return nil
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
		a.observeFailure(result.Err())
		return false
	}
	a.observeSuccess()
	return result.Val() > 0
}

// Delete 删除缓存
func (a *RedisCacheAdapter) Delete(key string) error {
	if a.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()
	fullKey := a.buildKey(key)
	if err := a.client.Del(ctx, fullKey).Err(); err != nil {
		a.observeFailure(err)
		return nil
	}
	a.observeSuccess()
	return nil
}

func (a *RedisCacheAdapter) observeSuccess() {
	if a == nil {
		return
	}
	if a.observer != nil {
		a.observer.ObserveFamilySuccess("sdk_token")
		return
	}
	observability.ObserveFamilySuccess("apiserver", "sdk_token")
}

func (a *RedisCacheAdapter) observeFailure(err error) {
	if a == nil {
		return
	}
	if a.observer != nil {
		a.observer.ObserveFamilyFailure("sdk_token", err)
		return
	}
	observability.ObserveFamilyFailure("apiserver", "sdk_token", err)
}
