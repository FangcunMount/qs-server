package object

import (
	"context"
	"errors"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

var ErrCoalescerRequired = errors.New("cache object read-through requires a coalescer when singleflight is enabled")

type ReadThroughOptions[T any] struct {
	Capability       sharedcache.Capability
	CacheKey         string
	PolicyProvider   sharedcache.PolicyProvider
	Observer         sharedcache.Observer
	Coalescer        loadguard.Coalescer
	Store            *Store[T]
	Load             func(context.Context) (*T, error)
	CacheNegative    bool
	AsyncSetCached   bool
	AsyncSetNegative bool
}

func ReadThrough[T any](ctx context.Context, opts ReadThroughOptions[T]) (*T, error) {
	policy := sharedcache.Policy{}
	if opts.PolicyProvider != nil {
		if effective, ok := opts.PolicyProvider.Resolve(opts.Capability); ok {
			policy = effective.Policy
		}
	}
	if opts.Store != nil {
		start := time.Now()
		cached, err := opts.Store.Get(ctx, opts.CacheKey)
		if err == nil {
			sharedcache.Observe(opts.Observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultHit, Duration: time.Since(start)})
			return cached, nil
		}
		if !errors.Is(err, sharedcache.ErrMiss) {
			sharedcache.Observe(opts.Observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultError, Duration: time.Since(start), Err: err})
		} else {
			sharedcache.Observe(opts.Observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultMiss, Duration: time.Since(start)})
		}
	}

	load := func() (any, error) {
		if opts.Load == nil {
			return (*T)(nil), nil
		}
		start := time.Now()
		value, err := opts.Load(ctx)
		sharedcache.Observe(opts.Observer, sharedcache.Event{Operation: sharedcache.OperationSourceLoad, Duration: time.Since(start), Err: err})
		return value, err
	}

	var value *T
	if policy.SingleflightEnabled(false) {
		coalescer := opts.Coalescer
		if coalescer == nil && opts.Store != nil {
			coalescer = opts.Store.Coalescer()
		}
		if coalescer == nil {
			return nil, ErrCoalescerRequired
		}
		loaded, err := coalescer.Do(ctx, string(opts.Capability)+":"+opts.CacheKey, load)
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			value, _ = loaded.(*T)
		}
	} else {
		loaded, err := load()
		if err != nil {
			return nil, err
		}
		value, _ = loaded.(*T)
	}

	if value == nil {
		if opts.CacheNegative && policy.NegativeEnabled(false) {
			write(ctx, opts.Observer, opts.AsyncSetNegative, func(writeCtx context.Context) error {
				if opts.Store == nil {
					return nil
				}
				return opts.Store.SetNegative(writeCtx, opts.CacheKey, policy)
			})
		}
		return nil, nil
	}

	write(ctx, opts.Observer, opts.AsyncSetCached, func(writeCtx context.Context) error {
		if opts.Store == nil {
			return nil
		}
		return opts.Store.Set(writeCtx, opts.CacheKey, value, policy)
	})
	return value, nil
}

func write(ctx context.Context, observer sharedcache.Observer, async bool, fn func(context.Context) error) {
	run := func(writeCtx context.Context) {
		start := time.Now()
		err := fn(writeCtx)
		result := sharedcache.ResultOK
		if err != nil {
			result = sharedcache.ResultError
		}
		sharedcache.Observe(observer, sharedcache.Event{Operation: sharedcache.OperationSet, Result: result, Duration: time.Since(start), Err: err})
	}
	if async {
		go run(context.Background())
		return
	}
	run(ctx)
}
