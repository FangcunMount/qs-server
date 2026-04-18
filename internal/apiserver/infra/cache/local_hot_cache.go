package cache

import (
	"container/list"
	"sync"
	"time"
)

type localHotCacheEntry[T any] struct {
	key    string
	value  T
	expire time.Time
}

// LocalHotCache 提供进程内短 TTL + 有界容量的热点缓存。
type LocalHotCache[T any] struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	ll         *list.List
	items      map[string]*list.Element
}

func NewLocalHotCache[T any](ttl time.Duration, maxEntries int) *LocalHotCache[T] {
	if ttl <= 0 || maxEntries <= 0 {
		return nil
	}
	return &LocalHotCache[T]{
		ttl:        ttl,
		maxEntries: maxEntries,
		ll:         list.New(),
		items:      make(map[string]*list.Element, maxEntries),
	}
}

func (c *LocalHotCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil || key == "" {
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return zero, false
	}
	entry := elem.Value.(*localHotCacheEntry[T])
	if time.Now().After(entry.expire) {
		c.removeElement(elem)
		return zero, false
	}
	c.ll.MoveToFront(elem)
	return entry.value, true
}

func (c *LocalHotCache[T]) Set(key string, value T) {
	if c == nil || key == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*localHotCacheEntry[T])
		entry.value = value
		entry.expire = time.Now().Add(c.ttl)
		c.ll.MoveToFront(elem)
		return
	}

	elem := c.ll.PushFront(&localHotCacheEntry[T]{
		key:    key,
		value:  value,
		expire: time.Now().Add(c.ttl),
	})
	c.items[key] = elem
	for c.ll.Len() > c.maxEntries {
		c.removeElement(c.ll.Back())
	}
}

func (c *LocalHotCache[T]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element, c.maxEntries)
}

func (c *LocalHotCache[T]) removeElement(elem *list.Element) {
	if c == nil || elem == nil {
		return
	}
	c.ll.Remove(elem)
	entry := elem.Value.(*localHotCacheEntry[T])
	delete(c.items, entry.key)
}
