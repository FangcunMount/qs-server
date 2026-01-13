package cache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// MyAssessmentListCacheTTL 默认 TTL（需要防爆且避免陈旧）
var MyAssessmentListCacheTTL = 10 * time.Minute

// MyAssessmentListCache “我的测评列表”缓存
// 采用懒加载：先查缓存，miss 时查询后写入；写路径做前缀删。
// 存储值为调用方提供的任意结构（通常为 AssessmentListResult）的 JSON。
type MyAssessmentListCache struct {
	cache      Cache
	keyBuilder *CacheKeyBuilder
	ttl        time.Duration
}

// NewMyAssessmentListCache 创建实例
func NewMyAssessmentListCache(c Cache) *MyAssessmentListCache {
	if c == nil {
		return nil
	}
	return &MyAssessmentListCache{
		cache:      c,
		keyBuilder: NewCacheKeyBuilder(),
		ttl:        MyAssessmentListCacheTTL,
	}
}

// Get 读取缓存并解码到 dest（指针）
func (c *MyAssessmentListCache) Get(ctx context.Context, userID uint64, page, pageSize int, status string, dest interface{}) error {
	if c == nil || c.cache == nil {
		return ErrCacheNotFound
	}

	key := c.buildKey(userID, page, pageSize, status)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Set 写入缓存（value 将被 JSON 序列化）
func (c *MyAssessmentListCache) Set(ctx context.Context, userID uint64, page, pageSize int, status string, value interface{}) {
	if c == nil || c.cache == nil || value == nil {
		return
	}

	key := c.buildKey(userID, page, pageSize, status)
	data, err := json.Marshal(value)
	if err != nil {
		return
	}

	_ = c.cache.Set(ctx, key, data, JitterTTL(c.ttl))
}

// Invalidate 删除某个用户所有列表缓存（按前缀）
func (c *MyAssessmentListCache) Invalidate(ctx context.Context, userID uint64) {
	if c == nil || c.cache == nil {
		return
	}
	pattern := c.keyBuilder.BuildAssessmentListKey(userID, "*")
	_ = c.cache.DeletePattern(ctx, pattern)
}

func (c *MyAssessmentListCache) buildKey(userID uint64, page, pageSize int, status string) string {
	raw := fmt.Sprintf("status=%s&page=%d&page_size=%d", status, page, pageSize)
	hash := sha1.Sum([]byte(raw))
	suffix := ":" + hex.EncodeToString(hash[:])[:8]
	return c.keyBuilder.BuildAssessmentListKey(userID, suffix)
}
