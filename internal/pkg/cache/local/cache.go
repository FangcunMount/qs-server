package local

import (
	"strings"
	"sync"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

// Options 进程内 TTL 缓存配置。
type Options struct {
	TTL            time.Duration
	TTLProvider    func() time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
	OnEntries      func(int)
	OnEviction     func(EvictionReason)
}

type EvictionReason string

const (
	EvictionReasonFIFO     EvictionReason = "fifo"
	EvictionReasonTTL      EvictionReason = "ttl"
	EvictionReasonExplicit EvictionReason = "explicit"
	EvictionReasonSignal   EvictionReason = "signal"
)

type StatsSnapshot struct {
	Entries           int    `json:"entries"`
	MaxEntries        int    `json:"max_entries"`
	Hits              uint64 `json:"hits"`
	Misses            uint64 `json:"misses"`
	FIFOEvictions     uint64 `json:"fifo_evictions"`
	TTLExpirations    uint64 `json:"ttl_expirations"`
	ExplicitDeletions uint64 `json:"explicit_deletions"`
	SignalDeletions   uint64 `json:"signal_deletions"`
}

type BucketSnapshot struct {
	Bucket string        `json:"bucket"`
	Stats  StatsSnapshot `json:"stats"`
}

func (o Options) withDefaults(defaultTTL time.Duration, defaultEntries int) Options {
	if o.TTL <= 0 {
		o.TTL = defaultTTL
	}
	if o.MaxEntries <= 0 {
		o.MaxEntries = defaultEntries
	}
	return o
}

type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// Cache 泛型进程内 TTL 缓存（FIFO 淘汰）。
type Cache[T any] struct {
	mu                sync.RWMutex
	opts              Options
	clone             func(T) T
	items             map[string]cacheEntry[T]
	order             []string
	hits              uint64
	misses            uint64
	fifoEvictions     uint64
	ttlExpirations    uint64
	explicitDeletions uint64
	signalDeletions   uint64
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
		entry, ok = c.items[key]
		if !ok {
			c.mu.Unlock()
			c.recordMiss()
			return zero, false
		}
		if !entry.expiresAt.After(now) {
			delete(c.items, key)
			c.removeOrderKey(key)
			c.recordEvictionLocked(EvictionReasonTTL)
			c.recordEntriesLocked()
			c.mu.Unlock()
			c.recordMiss()
			return zero, false
		}
		cloned := c.clone(entry.value)
		c.mu.Unlock()
		c.recordHit()
		return cloned, true
	}

	c.recordHit()
	return c.clone(entry.value), true
}

func (c *Cache[T]) Set(key string, value T) {
	key = strings.TrimSpace(key)
	if c == nil || key == "" {
		return
	}

	ttl := c.opts.TTL
	if c.opts.TTLProvider != nil {
		if current := c.opts.TTLProvider(); current > 0 {
			ttl = current
		}
	}
	entry := cacheEntry[T]{
		value:     c.clone(value),
		expiresAt: time.Now().Add(sharedcache.JitterTTL(ttl, c.opts.TTLJitterRatio)),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; !exists {
		c.order = append(c.order, key)
	}
	c.items[key] = entry
	c.evictIfNeeded()
	c.recordEntriesLocked()
}

func (c *Cache[T]) Delete(key string) {
	c.DeleteWithReason(key, EvictionReasonExplicit)
}

func (c *Cache[T]) DeleteWithReason(key string, reason EvictionReason) {
	key = strings.TrimSpace(key)
	if c == nil || key == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists {
		return
	}
	delete(c.items, key)
	c.removeOrderKey(key)
	c.recordEvictionLocked(reason)
	c.recordEntriesLocked()
}

// DeletePrefix 删除 key 等于 prefix 或以 prefix 为前缀的全部条目。
func (c *Cache[T]) DeletePrefix(prefix string) {
	c.DeletePrefixWithReason(prefix, EvictionReasonExplicit)
}

func (c *Cache[T]) DeletePrefixWithReason(prefix string, reason EvictionReason) {
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
			c.recordEvictionLocked(reason)
		}
	}
	c.recordEntriesLocked()
}

func (c *Cache[T]) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *Cache[T]) RuntimeSnapshot() StatsSnapshot {
	if c == nil {
		return StatsSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return StatsSnapshot{
		Entries: len(c.items), MaxEntries: c.opts.MaxEntries, Hits: c.hits, Misses: c.misses,
		FIFOEvictions: c.fifoEvictions, TTLExpirations: c.ttlExpirations,
		ExplicitDeletions: c.explicitDeletions, SignalDeletions: c.signalDeletions,
	}
}

func (c *Cache[T]) recordHit() {
	if c.opts.OnHit != nil {
		c.opts.OnHit()
	}
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
}

func (c *Cache[T]) recordMiss() {
	if c.opts.OnMiss != nil {
		c.opts.OnMiss()
	}
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

func (c *Cache[T]) evictIfNeeded() {
	for len(c.items) > c.opts.MaxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
		c.recordEvictionLocked(EvictionReasonFIFO)
	}
}

func (c *Cache[T]) recordEvictionLocked(reason EvictionReason) {
	switch reason {
	case EvictionReasonFIFO:
		c.fifoEvictions++
	case EvictionReasonTTL:
		c.ttlExpirations++
	case EvictionReasonSignal:
		c.signalDeletions++
	default:
		c.explicitDeletions++
	}
	if c.opts.OnEviction != nil {
		c.opts.OnEviction(reason)
	}
}

func (c *Cache[T]) recordEntriesLocked() {
	if c.opts.OnEntries != nil {
		c.opts.OnEntries(len(c.items))
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
