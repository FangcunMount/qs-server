package cachequery

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
)

const defaultScaleHotListCacheTTL = 3 * time.Minute

// PublishedScaleHotListCache 缓存热门量表列表 JSON 响应。
type PublishedScaleHotListCache struct {
	entry      cacheentry.Cache
	payload    *cacheentry.PayloadStore
	keyBuilder *keyspace.Builder
	policy     cachepolicy.CachePolicy
	memory     *LocalHotCache[[]byte]
}

func NewPublishedScaleHotListCacheWithPolicyAndKeyBuilder(
	entry cacheentry.Cache,
	keyBuilder *keyspace.Builder,
	policy cachepolicy.CachePolicy,
) scalelistcache.HotListCache {
	if entry == nil {
		return nil
	}
	if keyBuilder == nil {
		panic("cache key builder is required")
	}

	return &PublishedScaleHotListCache{
		entry:      entry,
		payload:    cacheentry.NewPayloadStore(entry, cachepolicy.PolicyScaleList, policy),
		keyBuilder: keyBuilder,
		policy:     policy,
		memory:     NewLocalHotCache[[]byte](30*time.Second, 16),
	}
}

func (c *PublishedScaleHotListCache) Get(ctx context.Context, limit, windowDays int) ([]byte, bool) {
	if c == nil || c.payload == nil {
		return nil, false
	}

	memKey := c.buildMemoryKey(limit, windowDays)
	if cached, ok := c.getMemory(memKey); ok {
		return cached, true
	}

	key := c.keyBuilder.BuildScaleHotListKey(limit, windowDays)
	data, err := c.payload.Get(ctx, key)
	if err != nil {
		if err == cacheentry.ErrCacheNotFound {
			observability.ObserveFamilySuccess("apiserver", "static_meta")
		} else {
			observability.ObserveFamilyFailure("apiserver", "static_meta", err)
		}
		return nil, false
	}
	observability.ObserveFamilySuccess("apiserver", "static_meta")
	c.setMemory(memKey, data)
	return data, true
}

func (c *PublishedScaleHotListCache) Set(ctx context.Context, limit, windowDays int, payload []byte) error {
	if c == nil || c.payload == nil || len(payload) == 0 {
		return nil
	}

	key := c.keyBuilder.BuildScaleHotListKey(limit, windowDays)
	if err := c.payload.Set(ctx, key, payload, c.policy.TTLOr(defaultScaleHotListCacheTTL)); err != nil {
		observability.ObserveFamilyFailure("apiserver", "static_meta", err)
		return err
	}
	observability.ObserveFamilySuccess("apiserver", "static_meta")
	c.setMemory(c.buildMemoryKey(limit, windowDays), payload)
	return nil
}

func (c *PublishedScaleHotListCache) buildMemoryKey(limit, windowDays int) string {
	return fmt.Sprintf("limit=%d:window_days=%d", limit, windowDays)
}

func (c *PublishedScaleHotListCache) getMemory(key string) ([]byte, bool) {
	if c == nil || c.memory == nil {
		return nil, false
	}
	return c.memory.Get(key)
}

func (c *PublishedScaleHotListCache) setMemory(key string, payload []byte) {
	if c == nil || c.memory == nil || len(payload) == 0 {
		return
	}
	copied := append([]byte(nil), payload...)
	c.memory.Set(key, copied)
}
