package loadguard

import (
	"context"
	"reflect"

	"golang.org/x/sync/singleflight"
)

// Coalescer 合并并发回源请求。
type Coalescer interface {
	Do(ctx context.Context, key string, fn func() (any, error)) (any, error)
	Forget(key string)
}

// NoopCoalescer 不合并并发回源。
type NoopCoalescer struct{}

func (NoopCoalescer) Do(_ context.Context, _ string, fn func() (any, error)) (any, error) {
	return fn()
}

func (NoopCoalescer) Forget(string) {}

// SingleflightCoalescer 使用 singleflight 合并同一 key 的并发回源。
type SingleflightCoalescer struct {
	sf singleflight.Group
}

func (c *SingleflightCoalescer) Do(_ context.Context, key string, fn func() (any, error)) (any, error) {
	value, err, _ := c.sf.Do(key, func() (any, error) {
		v, callErr := fn()
		if callErr == nil && isNilAny(v) {
			c.sf.Forget(key)
		}
		return v, callErr
	})
	return value, err
}

func isNilAny(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Interface, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

// Forget 清除 key 的合并缓存（用于不应缓存的 miss 结果）。
func (c *SingleflightCoalescer) Forget(key string) {
	if c == nil {
		return
	}
	c.sf.Forget(key)
}

// NewCoalescer 按策略构造合并器。
func NewCoalescer(singleflight bool) Coalescer {
	if singleflight {
		return &SingleflightCoalescer{}
	}
	return NoopCoalescer{}
}
