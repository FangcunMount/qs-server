package cachequery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

// VersionedQueryCache owns the version-token + versioned-key query/list cache path.
type VersionedQueryCache struct {
	version  VersionTokenStore
	policy   cachepolicy.CachePolicy
	key      cachepolicy.CachePolicyKey
	ttl      time.Duration
	memory   *LocalHotCache[[]byte]
	observer FamilyObserver
	payload  *cacheentry.PayloadStore
}

func NewVersionedQueryCache(
	cache cacheentry.Cache,
	versionStore VersionTokenStore,
	policyKey cachepolicy.CachePolicyKey,
	policy cachepolicy.CachePolicy,
	ttl time.Duration,
	memory *LocalHotCache[[]byte],
) *VersionedQueryCache {
	return NewVersionedQueryCacheWithObserver(cache, versionStore, policyKey, policy, ttl, memory, nil)
}

func NewVersionedQueryCacheWithObserver(
	cache cacheentry.Cache,
	versionStore VersionTokenStore,
	policyKey cachepolicy.CachePolicyKey,
	policy cachepolicy.CachePolicy,
	ttl time.Duration,
	memory *LocalHotCache[[]byte],
	observer FamilyObserver,
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
		payload:  cacheentry.NewPayloadStore(cache, policyKey, policy),
	}
}

func (c *VersionedQueryCache) CurrentVersion(ctx context.Context, versionKey string) (uint64, error) {
	if c == nil || c.version == nil {
		return 0, cacheentry.ErrCacheNotFound
	}
	return c.version.Current(ctx, versionKey)
}

func (c *VersionedQueryCache) Get(ctx context.Context, versionKey string, buildDataKey func(uint64) string, dest interface{}) error {
	if c == nil || c.payload == nil || buildDataKey == nil {
		return cacheentry.ErrCacheNotFound
	}
	family := string(cachepolicy.FamilyFor(c.key))

	version, err := c.CurrentVersion(ctx, versionKey)
	if err != nil {
		observability.ObserveCacheGet(family, string(c.key), "miss")
		return cacheentry.ErrCacheNotFound
	}
	key := buildDataKey(version)

	if c.memory != nil {
		if data, ok := c.memory.Get(key); ok {
			observability.ObserveCacheGet(family, string(c.key), "hit")
			cacheentry.ObservePayload(c.key, len(data), len(data))
			if err := json.Unmarshal(data, dest); err != nil {
				observability.ObserveCacheGet(family, string(c.key), "error")
				return cacheentry.ErrCacheNotFound
			}
			return nil
		}
	}

	start := time.Now()
	data, err := c.payload.Get(ctx, key)
	observability.ObserveCacheOperationDuration(family, string(c.key), "get", time.Since(start))
	if err != nil {
		if err == cacheentry.ErrCacheNotFound {
			observability.ObserveCacheGet(family, string(c.key), "miss")
			c.observeSuccess(family)
		} else {
			observability.ObserveCacheGet(family, string(c.key), "error")
			c.observeFailure(family, err)
			observability.ObserveCacheGet(family, string(c.key), "miss")
		}
		return cacheentry.ErrCacheNotFound
	}
	if err := json.Unmarshal(data, dest); err != nil {
		observability.ObserveCacheGet(family, string(c.key), "error")
		c.observeFailure(family, err)
		observability.ObserveCacheGet(family, string(c.key), "miss")
		return cacheentry.ErrCacheNotFound
	}
	observability.ObserveCacheGet(family, string(c.key), "hit")
	c.observeSuccess(family)
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
		observability.ObserveCacheOperationDuration(family, string(c.key), "set", time.Since(start))
		observability.ObserveCacheWrite(family, string(c.key), "set", "error")
		c.observeFailure(family, err)
		return
	}
	observability.ObserveCacheOperationDuration(family, string(c.key), "set", time.Since(start))
	observability.ObserveCacheWrite(family, string(c.key), "set", "ok")
	c.observeSuccess(family)
}

func (c *VersionedQueryCache) Invalidate(ctx context.Context, versionKey string) error {
	if c == nil || c.version == nil {
		return nil
	}
	_, err := c.version.Bump(ctx, versionKey)
	if err != nil {
		cacheentry.ObserveInvalidate(c.key, "error")
		return err
	}
	cacheentry.ObserveInvalidate(c.key, "ok")
	return nil
}

func (c *VersionedQueryCache) observeSuccess(family string) {
	if c != nil && c.observer != nil {
		c.observer.ObserveFamilySuccess(family)
	}
}

func (c *VersionedQueryCache) observeFailure(family string, err error) {
	if c != nil && c.observer != nil {
		c.observer.ObserveFamilyFailure(family, err)
	}
}
