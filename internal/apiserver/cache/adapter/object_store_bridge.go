package cache

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	objectcache "github.com/FangcunMount/qs-server/internal/pkg/cache/object"
)

type CacheEntryCodec[T any] = objectcache.Codec[T]
type ObjectCacheStore[T any] = objectcache.Store[T]

type ObjectCacheStoreOptions[T any] struct {
	Cache       sharedcache.Store
	PolicyKey   cachepolicy.CachePolicyKey
	Policy      sharedcache.Policy
	TTL         time.Duration
	NegativeTTL time.Duration
	Codec       CacheEntryCodec[T]
}

func NewObjectCacheStore[T any](opts ObjectCacheStoreOptions[T]) *ObjectCacheStore[T] {
	return objectcache.NewStore(objectcache.StoreOptions[T]{
		Store:       opts.Cache,
		Policy:      opts.Policy,
		TTL:         opts.TTL,
		NegativeTTL: opts.NegativeTTL,
		Codec:       opts.Codec,
		Observer:    newCapabilityObserver(opts.PolicyKey, nil),
		Coalescer:   newCapabilityCoalescer(opts.Policy),
	})
}
