package loadguard

import "sync"

// StaleStore 保存进程内陈旧值，供回源失败时降级返回。
type StaleStore[K comparable, V any] interface {
	Remember(K, V)
	Load(K) (V, bool)
}

// DisabledStaleStore 禁用陈旧降级。
type DisabledStaleStore[K comparable, V any] struct{}

func (DisabledStaleStore[K, V]) Remember(K, V) {}

func (DisabledStaleStore[K, V]) Load(K) (V, bool) {
	var zero V
	return zero, false
}

// MemoryStaleStore 使用 sync.Map 保存陈旧值副本。
type MemoryStaleStore[K comparable, V any] struct {
	store sync.Map
	clone func(V) V
}

// NewMemoryStaleStore 创建进程内陈旧存储；clone 为 nil 时直接保存值副本引用。
func NewMemoryStaleStore[K comparable, V any](clone func(V) V) *MemoryStaleStore[K, V] {
	if clone == nil {
		clone = func(v V) V { return v }
	}
	return &MemoryStaleStore[K, V]{clone: clone}
}

func (s *MemoryStaleStore[K, V]) Remember(key K, value V) {
	s.store.Store(key, s.clone(value))
}

func (s *MemoryStaleStore[K, V]) Load(key K) (V, bool) {
	var zero V
	raw, ok := s.store.Load(key)
	if !ok {
		return zero, false
	}
	value, ok := raw.(V)
	if !ok {
		return zero, false
	}
	return s.clone(value), true
}
