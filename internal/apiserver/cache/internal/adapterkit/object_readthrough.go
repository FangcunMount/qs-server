package adapterkit

import (
	"context"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	objectcache "github.com/FangcunMount/qs-server/internal/pkg/cache/object"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

type ObjectReadThroughOptions[T any] struct {
	PolicyKey        sharedcache.Capability
	CacheKey         string
	PolicyProvider   sharedcache.PolicyProvider
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
		PolicyProvider:   opts.PolicyProvider,
		Observer:         NewCapabilityObserver(opts.PolicyKey, opts.Observer),
		Store:            opts.Store,
		Load:             opts.Load,
		CacheNegative:    opts.CacheNegative,
		AsyncSetCached:   opts.AsyncSetCached,
		AsyncSetNegative: opts.AsyncSetNegative,
	})
}
