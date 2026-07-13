package adapterkit

import (
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	objectcache "github.com/FangcunMount/qs-server/internal/pkg/cache/object"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type CacheEntryCodec[T any] = objectcache.Codec[T]
type ObjectCacheStore[T any] = objectcache.Store[T]

type ObjectCacheStoreOptions[T any] struct {
	Cache     sharedcache.Store
	PolicyKey sharedcache.Capability
	Codec     CacheEntryCodec[T]
}

func NewObjectCacheStore[T any](opts ObjectCacheStoreOptions[T]) *ObjectCacheStore[T] {
	return objectcache.NewStore(objectcache.StoreOptions[T]{
		Store: opts.Cache, Codec: opts.Codec,
		Observer:  NewCapabilityObserver(opts.PolicyKey, nil),
		Coalescer: loadguard.NewCoalescer(true),
	})
}
