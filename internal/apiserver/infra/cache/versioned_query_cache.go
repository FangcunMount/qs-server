package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

// VersionedQueryCache 封装 version token + versioned key 的 query/list cache 主路径。
type VersionedQueryCache struct {
	version  VersionTokenStore
	policy   cachepolicy.CachePolicy
	key      cachepolicy.CachePolicyKey
	ttl      time.Duration
	memory   *LocalHotCache[[]byte]
	observer *Observer
	payload  *cachePayloadStore
}

func NewVersionedQueryCache(
	cache Cache,
	versionStore VersionTokenStore,
	policyKey cachepolicy.CachePolicyKey,
	policy cachepolicy.CachePolicy,
	ttl time.Duration,
	memory *LocalHotCache[[]byte],
) *VersionedQueryCache {
	return NewVersionedQueryCacheWithObserver(cache, versionStore, policyKey, policy, ttl, memory, nil)
}

func NewVersionedQueryCacheWithObserver(
	cache Cache,
	versionStore VersionTokenStore,
	policyKey cachepolicy.CachePolicyKey,
	policy cachepolicy.CachePolicy,
	ttl time.Duration,
	memory *LocalHotCache[[]byte],
	observer *Observer,
) *VersionedQueryCache {
	if cache == nil || versionStore == nil {
		return nil
	}
	return &VersionedQueryCache{
		version:  versionStore,
		policy:   policy,
		key:      policyKey,
		ttl:      ttl,
		memory:   memory,
		observer: observer,
		payload:  newCachePayloadStore(cache, policyKey, policy),
	}
}

func (c *VersionedQueryCache) CurrentVersion(ctx context.Context, versionKey string) (uint64, error) {
	if c == nil || c.version == nil {
		return 0, ErrCacheNotFound
	}
	return c.version.Current(ctx, versionKey)
}

func (c *VersionedQueryCache) Get(ctx context.Context, versionKey string, buildDataKey func(uint64) string, dest interface{}) error {
	if c == nil || c.payload == nil || buildDataKey == nil {
		return ErrCacheNotFound
	}
	family := string(cachepolicy.FamilyFor(c.key))

	version, err := c.CurrentVersion(ctx, versionKey)
	if err != nil {
		cacheobservability.ObserveCacheGet(family, string(c.key), "miss")
		return ErrCacheNotFound
	}
	key := buildDataKey(version)

	if c.memory != nil {
		if data, ok := c.memory.Get(key); ok {
			cacheobservability.ObserveCacheGet(family, string(c.key), "hit")
			observePayload(c.key, len(data), len(data))
			if err := json.Unmarshal(data, dest); err != nil {
				cacheobservability.ObserveCacheGet(family, string(c.key), "error")
				return ErrCacheNotFound
			}
			return nil
		}
	}

	start := time.Now()
	data, err := c.payload.Get(ctx, key)
	cacheobservability.ObserveCacheOperationDuration(family, string(c.key), "get", time.Since(start))
	if err != nil {
		if err == ErrCacheNotFound {
			cacheobservability.ObserveCacheGet(family, string(c.key), "miss")
			c.observer.ObserveFamilySuccess(family)
		} else {
			cacheobservability.ObserveCacheGet(family, string(c.key), "error")
			c.observer.ObserveFamilyFailure(family, err)
			cacheobservability.ObserveCacheGet(family, string(c.key), "miss")
		}
		return ErrCacheNotFound
	}
	if err := json.Unmarshal(data, dest); err != nil {
		cacheobservability.ObserveCacheGet(family, string(c.key), "error")
		c.observer.ObserveFamilyFailure(family, err)
		cacheobservability.ObserveCacheGet(family, string(c.key), "miss")
		return ErrCacheNotFound
	}
	cacheobservability.ObserveCacheGet(family, string(c.key), "hit")
	c.observer.ObserveFamilySuccess(family)
	if c.memory != nil {
		c.memory.Set(key, data)
	}
	return nil
}

func (c *VersionedQueryCache) Set(ctx context.Context, versionKey string, buildDataKey func(uint64) string, value interface{}) {
	if c == nil || c.payload == nil || buildDataKey == nil || value == nil {
		return
	}
	family := string(cachepolicy.FamilyFor(c.key))

	version, err := c.CurrentVersion(ctx, versionKey)
	if err != nil {
		return
	}
	key := buildDataKey(version)
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}

	if c.memory != nil {
		c.memory.Set(key, raw)
	}

	start := time.Now()
	if err := c.payload.Set(ctx, key, raw, c.policy.TTLOr(c.ttl)); err != nil {
		cacheobservability.ObserveCacheOperationDuration(family, string(c.key), "set", time.Since(start))
		cacheobservability.ObserveCacheWrite(family, string(c.key), "set", "error")
		c.observer.ObserveFamilyFailure(family, err)
		return
	}
	cacheobservability.ObserveCacheOperationDuration(family, string(c.key), "set", time.Since(start))
	cacheobservability.ObserveCacheWrite(family, string(c.key), "set", "ok")
	c.observer.ObserveFamilySuccess(family)
}

func (c *VersionedQueryCache) Invalidate(ctx context.Context, versionKey string) error {
	if c == nil || c.version == nil {
		return nil
	}
	_, err := c.version.Bump(ctx, versionKey)
	if err != nil {
		observeInvalidate(c.key, "error")
		return err
	}
	observeInvalidate(c.key, "ok")
	return nil
}
