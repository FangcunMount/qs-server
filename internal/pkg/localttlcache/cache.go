package localttlcache

import (
	"strings"
	"sync"
	"time"
)

// Options 进程内 TTL 缓存配置。
type Options struct {
	TTL        time.Duration
	MaxEntries int
}

type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// Cache 泛型进程内 TTL 缓存（FIFO 淘汰）。
type Cache[T any] struct {
	mu     sync.RWMutex
	opts   Options
	clone  func(T) T
	items  map[string]cacheEntry[T]
	order  []string
	hits   uint64
	misses uint64
}

// New 创建进程内 TTL 缓存；clone 在 Get/Set 时隔离调用方修改。
func New[T any](opts Options, clone func(T) T) *Cache[T] {
	if opts.TTL <= 0 {
		opts.TTL = 180 * time.Second
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 256
	}
	if clone == nil {
		clone = func(v T) T { return v }
	}
	return &Cache[T]{
		opts:  opts,
		clone: clone,
		items: make(map[string]cacheEntry[T]),
		order: make([]string, 0, opts.MaxEntries),
	}
}

func (c *Cache[T]) Get(key string) (T, bool) {
	var zero T
	key = strings.TrimSpace(key)
	if c == nil || key == "" {
		return zero, false
	}

	now := time.Now()
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		c.recordMiss()
		return zero, false
	}
	if !entry.expiresAt.After(now) {
		c.mu.Lock()
		delete(c.items, key)
		c.removeOrderKey(key)
		c.mu.Unlock()
		c.recordMiss()
		return zero, false
	}

	c.recordHit()
	return c.clone(entry.value), true
}

func (c *Cache[T]) Set(key string, value T) {
	key = strings.TrimSpace(key)
	if c == nil || key == "" {
		return
	}

	entry := cacheEntry[T]{
		value:     c.clone(value),
		expiresAt: time.Now().Add(c.opts.TTL),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; !exists {
		c.order = append(c.order, key)
	}
	c.items[key] = entry
	c.evictIfNeeded()
}

func (c *Cache[T]) Delete(key string) {
	key = strings.TrimSpace(key)
	if c == nil || key == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	c.removeOrderKey(key)
}

// DeletePrefix 删除 key 等于 prefix 或以 prefix 为前缀的全部条目。
func (c *Cache[T]) DeletePrefix(prefix string) {
	prefix = strings.TrimSpace(prefix)
	if c == nil || prefix == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if key == prefix || strings.HasPrefix(key, prefix) {
			delete(c.items, key)
			c.removeOrderKey(key)
		}
	}
}

func (c *Cache[T]) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *Cache[T]) recordHit() {
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
}

func (c *Cache[T]) recordMiss() {
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

func (c *Cache[T]) evictIfNeeded() {
	for len(c.items) > c.opts.MaxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

func (c *Cache[T]) removeOrderKey(key string) {
	for i, existing := range c.order {
		if existing != key {
			continue
		}
		c.order = append(c.order[:i], c.order[i+1:]...)
		return
	}
}
