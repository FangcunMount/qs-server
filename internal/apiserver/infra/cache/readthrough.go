package cache

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

// ReadThroughOptions 描述一次统一的缓存读穿透流程。
type ReadThroughOptions[T any] struct {
	PolicyKey         cachepolicy.CachePolicyKey
	CacheKey          string
	Policy            cachepolicy.CachePolicy
	Observer          *Observer
	GetCached         func(context.Context) (*T, error)
	Load              func(context.Context) (*T, error)
	SetCached         func(context.Context, *T) error
	SetNegativeCached func(context.Context) error
	AsyncSetCached    bool
	AsyncSetNegative  bool
}

// ReadThrough 执行统一缓存读穿透：
// 1. 优先读缓存
// 2. miss 后按对象级 singleflight 回源
// 3. 回源成功后写回正向缓存或 negative sentinel
func ReadThrough[T any](ctx context.Context, opts ReadThroughOptions[T]) (*T, error) {
	family := string(cachepolicy.FamilyFor(opts.PolicyKey))
	policy := string(opts.PolicyKey)

	if opts.GetCached != nil {
		start := time.Now()
		cached, err := opts.GetCached(ctx)
		cacheobservability.ObserveCacheOperationDuration(family, policy, "get", time.Since(start))
		if err == nil {
			cacheobservability.ObserveCacheGet(family, policy, "hit")
			opts.Observer.ObserveFamilySuccess(family)
			return cached, nil
		}
		if err != ErrCacheNotFound {
			cacheobservability.ObserveCacheGet(family, policy, "error")
			opts.Observer.ObserveFamilyFailure(family, err)
		} else {
			opts.Observer.ObserveFamilySuccess(family)
		}
		cacheobservability.ObserveCacheGet(family, policy, "miss")
	}

	load := func() (*T, error) {
		if opts.Load == nil {
			return nil, nil
		}
		start := time.Now()
		value, err := opts.Load(ctx)
		cacheobservability.ObserveCacheOperationDuration(family, policy, "source_load", time.Since(start))
		return value, err
	}

	var (
		value *T
		err   error
	)
	if opts.Policy.SingleflightEnabled(false) {
		result, _, doErr := sharedSingleflightCoordinator().Do(opts.PolicyKey, opts.CacheKey, func() (interface{}, error) {
			return load()
		})
		if doErr != nil {
			return nil, doErr
		}
		if result != nil {
			value, _ = result.(*T)
		}
	} else {
		value, err = load()
		if err != nil {
			return nil, err
		}
	}

	if value == nil {
		if opts.Policy.NegativeEnabled(false) && opts.SetNegativeCached != nil {
			writeNegative := func(writeCtx context.Context) {
				start := time.Now()
				err := opts.SetNegativeCached(writeCtx)
				cacheobservability.ObserveCacheOperationDuration(family, policy, "set", time.Since(start))
				if err != nil {
					cacheobservability.ObserveCacheWrite(family, policy, "set", "error")
					opts.Observer.ObserveFamilyFailure(family, err)
					return
				}
				opts.Observer.ObserveFamilySuccess(family)
				cacheobservability.ObserveCacheWrite(family, policy, "set", "ok")
			}
			if opts.AsyncSetNegative {
				go writeNegative(context.Background())
			} else {
				writeNegative(ctx)
			}
		}
		return nil, nil
	}

	if opts.SetCached != nil {
		writeValue := func(writeCtx context.Context) {
			start := time.Now()
			err := opts.SetCached(writeCtx, value)
			cacheobservability.ObserveCacheOperationDuration(family, policy, "set", time.Since(start))
			if err != nil {
				cacheobservability.ObserveCacheWrite(family, policy, "set", "error")
				opts.Observer.ObserveFamilyFailure(family, err)
				return
			}
			opts.Observer.ObserveFamilySuccess(family)
			cacheobservability.ObserveCacheWrite(family, policy, "set", "ok")
		}
		if opts.AsyncSetCached {
			go writeValue(context.Background())
		} else {
			writeValue(ctx)
		}
	}

	return value, nil
}
