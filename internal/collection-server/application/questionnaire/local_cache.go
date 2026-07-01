package questionnaire

import (
	"strings"
	"sync"
	"time"
)

// LocalCacheOptions 进程内 L1 缓存配置。
type LocalCacheOptions struct {
	TTL        time.Duration
	MaxEntries int
}

// LocalCache 已发布问卷 REST DTO 的进程内 TTL 缓存。
type LocalCache struct {
	mu     sync.RWMutex
	opts   LocalCacheOptions
	items  map[string]localCacheEntry
	order  []string
	hits   uint64
	misses uint64
}

type localCacheEntry struct {
	value     *QuestionnaireResponse
	expiresAt time.Time
}

// NewLocalCache 创建进程内问卷详情缓存。
func NewLocalCache(opts LocalCacheOptions) *LocalCache {
	if opts.TTL <= 0 {
		opts.TTL = defaultLocalCacheTTLSeconds * time.Second
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 256
	}
	return &LocalCache{
		opts:  opts,
		items: make(map[string]localCacheEntry),
		order: make([]string, 0, opts.MaxEntries),
	}
}

func (c *LocalCache) Get(code, version string) (*QuestionnaireResponse, bool) {
	key := cacheKey(code, version)
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		c.recordMiss()
		return nil, false
	}
	if !entry.expiresAt.After(now) {
		c.mu.Lock()
		delete(c.items, key)
		c.removeOrderKey(key)
		c.mu.Unlock()
		c.recordMiss()
		return nil, false
	}

	c.recordHit()
	return cloneResponse(entry.value), true
}

func (c *LocalCache) Set(code, version string, value *QuestionnaireResponse) {
	if value == nil {
		return
	}

	key := cacheKey(code, version)
	entry := localCacheEntry{
		value:     cloneResponse(value),
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

func (c *LocalCache) Delete(code, version string) {
	code = strings.ToLower(strings.TrimSpace(code))
	version = strings.TrimSpace(version)

	c.mu.Lock()
	defer c.mu.Unlock()

	if version == "" {
		prefix := "published:" + code
		for key := range c.items {
			if key == prefix || strings.HasPrefix(key, prefix+":") {
				delete(c.items, key)
				c.removeOrderKey(key)
			}
		}
		return
	}

	key := cacheKey(code, version)
	delete(c.items, key)
	c.removeOrderKey(key)
}

func (c *LocalCache) Stats() (hits, misses uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *LocalCache) recordHit() {
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
}

func (c *LocalCache) recordMiss() {
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

func (c *LocalCache) evictIfNeeded() {
	for len(c.items) > c.opts.MaxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

func (c *LocalCache) removeOrderKey(key string) {
	for i, existing := range c.order {
		if existing != key {
			continue
		}
		c.order = append(c.order[:i], c.order[i+1:]...)
		return
	}
}
