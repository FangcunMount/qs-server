package catalogl1

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/localttlcache"
)

// DetailHooks 单桶详情 L1（问卷等）。
type DetailHooks[T any] struct {
	KeyFn  func(code, version string) string
	Clone  func(T) T
	Prefix string
}

// DetailCache 单桶详情 L1。
type DetailCache[T any] struct {
	hooks DetailHooks[T]
	inner *localttlcache.Cache[T]
}

// NewDetailCache 创建单桶详情 L1。
func NewDetailCache[T any](opts Options, hooks DetailHooks[T]) *DetailCache[T] {
	opts = opts.withDefaults(defaultTTL, 256)
	if hooks.Clone == nil {
		return &DetailCache[T]{hooks: hooks}
	}
	return &DetailCache[T]{
		hooks: hooks,
		inner: localttlcache.New(opts.localTTL(nil), hooks.Clone),
	}
}

func (c *DetailCache[T]) Get(code, version string) (T, bool) {
	var zero T
	if c == nil || c.inner == nil || c.hooks.KeyFn == nil {
		return zero, false
	}
	return c.inner.Get(c.hooks.KeyFn(code, version))
}

func (c *DetailCache[T]) Set(code, version string, value T) {
	if c == nil || c.inner == nil || c.hooks.KeyFn == nil || isNilValue(value) {
		return
	}
	c.inner.Set(c.hooks.KeyFn(code, version), value)
}

func (c *DetailCache[T]) Delete(code, version string) {
	if c == nil || c.inner == nil {
		return
	}
	code = strings.ToLower(strings.TrimSpace(code))
	version = strings.TrimSpace(version)
	if c.hooks.Prefix != "" && version == "" {
		c.inner.DeletePrefix(c.hooks.Prefix + code)
		return
	}
	if c.hooks.KeyFn != nil {
		c.inner.Delete(c.hooks.KeyFn(code, version))
	}
}

func (c *DetailCache[T]) EvictOnSignal(code, version string) {
	c.Delete(code, version)
}

func (c *DetailCache[T]) Stats() (hits, misses uint64) {
	if c == nil || c.inner == nil {
		return 0, 0
	}
	return c.inner.Stats()
}
