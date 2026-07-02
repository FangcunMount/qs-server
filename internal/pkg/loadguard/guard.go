package loadguard

import (
	"context"
	"fmt"
	"time"
)

// Guard 在 cache miss 路径上提供并发合并、回源超时与进程内 stale 降级。
type Guard[K comparable, V any] struct {
	policy    Policy
	coalescer Coalescer
	stale     StaleStore[K, V]
	onStale   func()
}

// New 构造读路径保护器。
func New[K comparable, V any](policy Policy, clone func(V) V, onStale func()) *Guard[K, V] {
	g := &Guard[K, V]{
		policy:    policy,
		coalescer: NewCoalescer(policy.Singleflight),
		onStale:   onStale,
	}
	if policy.StaleOnError {
		g.stale = NewMemoryStaleStore[K, V](clone)
	} else {
		g.stale = DisabledStaleStore[K, V]{}
	}
	return g
}

// RememberStale 显式记录陈旧值（例如缓存命中路径预热 stale）。
func (g *Guard[K, V]) RememberStale(key K, value V) {
	if g == nil || g.stale == nil {
		return
	}
	g.stale.Remember(key, value)
}

// LoadStale 读取进程内陈旧值。
func (g *Guard[K, V]) LoadStale(key K) (V, bool) {
	if g == nil || g.stale == nil {
		var zero V
		return zero, false
	}
	return g.stale.Load(key)
}

// Load 执行受保护的回源加载。
func (g *Guard[K, V]) Load(ctx context.Context, key K, loader func(context.Context) (V, error)) (V, error) {
	var zero V
	if g == nil || loader == nil {
		return zero, nil
	}

	load := func(loadCtx context.Context) (V, error) {
		timedCtx, cancel := g.withLoadTimeout(loadCtx)
		defer cancel()
		value, err := loader(timedCtx)
		if err != nil {
			if stale, ok := g.stale.Load(key); ok {
				if g.onStale != nil {
					g.onStale()
				}
				return stale, nil
			}
			return zero, err
		}
		g.stale.Remember(key, value)
		return value, nil
	}

	value, err := g.coalescer.Do(ctx, fmt.Sprint(key), func() (any, error) {
		return load(ctx)
	})
	if err != nil {
		return zero, err
	}
	typed, ok := value.(V)
	if !ok {
		return zero, nil
	}
	return typed, nil
}

func (g *Guard[K, V]) withLoadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if g == nil || g.policy.LoadTimeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= g.policy.LoadTimeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, g.policy.LoadTimeout)
}
