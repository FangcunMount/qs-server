package loadguard

import (
	"context"

	"golang.org/x/sync/singleflight"
)

// Coalescer 合并并发回源请求。
type Coalescer interface {
	Do(ctx context.Context, key string, fn func() (any, error)) (any, error)
}

// NoopCoalescer 不合并并发回源。
type NoopCoalescer struct{}

func (NoopCoalescer) Do(_ context.Context, _ string, fn func() (any, error)) (any, error) {
	return fn()
}

// SingleflightCoalescer 使用 singleflight 合并同一 key 的并发回源。
type SingleflightCoalescer struct {
	sf singleflight.Group
}

func (c *SingleflightCoalescer) Do(_ context.Context, key string, fn func() (any, error)) (any, error) {
	value, err, _ := c.sf.Do(key, fn)
	return value, err
}

// NewCoalescer 按策略构造合并器。
func NewCoalescer(singleflight bool) Coalescer {
	if singleflight {
		return &SingleflightCoalescer{}
	}
	return NoopCoalescer{}
}
