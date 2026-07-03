package cache

import (
	"context"
	"sync"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// SingleflightCoordinator 按对象策略维度管理 Coalescer，
// 避免所有缓存共享同一个全局 bucket。
type SingleflightCoordinator struct {
	mu         sync.Mutex
	coalescers map[cachepolicy.CachePolicyKey]loadguard.Coalescer
}

func NewSingleflightCoordinator() *SingleflightCoordinator {
	return &SingleflightCoordinator{
		coalescers: make(map[cachepolicy.CachePolicyKey]loadguard.Coalescer),
	}
}

func (c *SingleflightCoordinator) coalescer(policyKey cachepolicy.CachePolicyKey) loadguard.Coalescer {
	if c == nil {
		return loadguard.NewCoalescer(true)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.coalescers == nil {
		c.coalescers = make(map[cachepolicy.CachePolicyKey]loadguard.Coalescer)
	}
	if co, ok := c.coalescers[policyKey]; ok {
		return co
	}
	co := loadguard.NewCoalescer(true)
	c.coalescers[policyKey] = co
	return co
}

func (c *SingleflightCoordinator) Do(policyKey cachepolicy.CachePolicyKey, key string, fn func() (interface{}, error)) (interface{}, bool, error) {
	value, err := c.coalescer(policyKey).Do(context.Background(), key, fn)
	return value, false, err
}

var (
	defaultCoordinatorMu sync.RWMutex
	defaultCoordinator   *SingleflightCoordinator
	lazyDefaultOnce      sync.Once
	lazyDefault          *SingleflightCoordinator
)

// SetDefaultSingleflightCoordinator 由 container 注入进程级默认合并器。
func SetDefaultSingleflightCoordinator(c *SingleflightCoordinator) {
	defaultCoordinatorMu.Lock()
	defer defaultCoordinatorMu.Unlock()
	defaultCoordinator = c
}

func sharedSingleflightCoordinator() *SingleflightCoordinator {
	defaultCoordinatorMu.RLock()
	c := defaultCoordinator
	defaultCoordinatorMu.RUnlock()
	if c != nil {
		return c
	}
	lazyDefaultOnce.Do(func() {
		lazyDefault = NewSingleflightCoordinator()
	})
	return lazyDefault
}
