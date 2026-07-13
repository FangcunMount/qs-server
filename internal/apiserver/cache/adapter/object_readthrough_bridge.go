package cache

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	objectcache "github.com/FangcunMount/qs-server/internal/pkg/cache/object"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

type ObjectReadThroughOptions[T any] struct {
	PolicyKey        cachepolicy.CachePolicyKey
	CacheKey         string
	Policy           sharedcache.Policy
	Observer         *observability.ComponentObserver
	Store            *objectcache.Store[T]
	Load             func(context.Context) (*T, error)
	CacheNegative    bool
	AsyncSetCached   bool
	AsyncSetNegative bool
}

func ReadThroughObject[T any](ctx context.Context, opts ObjectReadThroughOptions[T]) (*T, error) {
	return objectcache.ReadThrough(ctx, objectcache.ReadThroughOptions[T]{
		Capability:       sharedcache.Capability(opts.PolicyKey),
		CacheKey:         opts.CacheKey,
		Policy:           opts.Policy,
		Observer:         newCapabilityObserver(opts.PolicyKey, opts.Observer),
		Store:            opts.Store,
		Load:             opts.Load,
		CacheNegative:    opts.CacheNegative,
		AsyncSetCached:   opts.AsyncSetCached,
		AsyncSetNegative: opts.AsyncSetNegative,
	})
}
