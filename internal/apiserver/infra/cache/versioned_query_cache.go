package cache

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
)

type VersionedQueryCache = cachequery.VersionedQueryCache

func NewVersionedQueryCache(
	cache Cache,
	versionStore VersionTokenStore,
	policyKey cachepolicy.CachePolicyKey,
	policy cachepolicy.CachePolicy,
	ttl time.Duration,
	memory *LocalHotCache[[]byte],
) *VersionedQueryCache {
	return cachequery.NewVersionedQueryCache(cache, versionStore, policyKey, policy, ttl, memory)
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
	return cachequery.NewVersionedQueryCacheWithObserver(cache, versionStore, policyKey, policy, ttl, memory, observer)
}
