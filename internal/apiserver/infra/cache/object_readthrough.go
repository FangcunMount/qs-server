package cache

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

// ObjectReadThroughOptions describes the repository object-cache read-through path.
type ObjectReadThroughOptions[T any] struct {
	PolicyKey        cachepolicy.CachePolicyKey
	CacheKey         string
	Policy           cachepolicy.CachePolicy
	Observer         *observability.ComponentObserver
	Runner           *ReadThroughRunner[T]
	Store            *ObjectCacheStore[T]
	Load             func(context.Context) (*T, error)
	CacheNegative    bool
	AsyncSetCached   bool
	AsyncSetNegative bool
}

// ReadThroughObject narrows repository decorators to key/load/store wiring while
// preserving the generic ReadThrough behavior.
func ReadThroughObject[T any](ctx context.Context, opts ObjectReadThroughOptions[T]) (*T, error) {
	readOpts := ReadThroughOptions[T]{
		PolicyKey: opts.PolicyKey,
		CacheKey:  opts.CacheKey,
		Policy:    opts.Policy,
		Observer:  opts.Observer,
		Runner:    opts.Runner,
		GetCached: func(ctx context.Context) (*T, error) {
			if opts.Store == nil {
				return nil, cacheentry.ErrCacheNotFound
			}
			return opts.Store.Get(ctx, opts.CacheKey)
		},
		Load: opts.Load,
		SetCached: func(ctx context.Context, value *T) error {
			if opts.Store == nil {
				return nil
			}
			return opts.Store.Set(ctx, opts.CacheKey, value)
		},
		AsyncSetCached:   opts.AsyncSetCached,
		AsyncSetNegative: opts.AsyncSetNegative,
	}
	if opts.CacheNegative {
		readOpts.SetNegativeCached = func(ctx context.Context) error {
			if opts.Store == nil {
				return nil
			}
			return opts.Store.SetNegative(ctx, opts.CacheKey)
		}
	}
	return ReadThrough(ctx, readOpts)
}
