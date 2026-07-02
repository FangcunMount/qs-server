package statistics

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// readGuard 为统计读路径提供 singleflight、回源超时与进程内 stale 降级。
type readGuard[T any] struct {
	opts    StatisticsReadGuardOptions
	sf      singleflight.Group
	stale   sync.Map
	clone   func(T) T
	onStale func()
}

func newReadGuard[T any](opts StatisticsReadGuardOptions, clone func(T) T, onStale func()) *readGuard[T] {
	if clone == nil {
		clone = func(v T) T { return v }
	}
	return &readGuard[T]{
		opts:    opts,
		clone:   clone,
		onStale: onStale,
	}
}

func (g *readGuard[T]) Load(ctx context.Context, key string, loader func(context.Context) (T, error)) (T, error) {
	var zero T
	if g == nil || loader == nil {
		return zero, nil
	}

	load := func(loadCtx context.Context) (T, error) {
		timedCtx, cancel := g.withLoadTimeout(loadCtx)
		defer cancel()
		value, err := loader(timedCtx)
		if err != nil {
			if stale, ok := g.loadStale(key); ok {
				if g.onStale != nil {
					g.onStale()
				}
				return stale, nil
			}
			return zero, err
		}
		g.rememberStale(key, value)
		return value, nil
	}

	if !g.opts.ServiceSingleflight {
		return load(ctx)
	}

	value, err, _ := g.sf.Do(key, func() (interface{}, error) {
		return load(ctx)
	})
	if err != nil {
		return zero, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, nil
	}
	return typed, nil
}

func (g *readGuard[T]) withLoadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if g == nil || g.opts.LoadTimeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= g.opts.LoadTimeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, g.opts.LoadTimeout)
}

func (g *readGuard[T]) rememberStale(key string, value T) {
	if g == nil || !g.opts.StaleOnTimeout {
		return
	}
	g.stale.Store(key, g.clone(value))
}

func (g *readGuard[T]) loadStale(key string) (T, bool) {
	var zero T
	if g == nil || !g.opts.StaleOnTimeout {
		return zero, false
	}
	raw, ok := g.stale.Load(key)
	if !ok {
		return zero, false
	}
	value, ok := raw.(T)
	if !ok {
		return zero, false
	}
	return g.clone(value), true
}
