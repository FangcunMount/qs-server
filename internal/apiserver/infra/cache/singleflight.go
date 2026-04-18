package cache

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// SingleflightCoordinator 按对象策略维度管理 singleflight 分组，
// 避免所有缓存共享同一个全局 bucket。
type SingleflightCoordinator struct {
	mu     sync.Mutex
	groups map[CachePolicyKey]*singleflight.Group
}

func NewSingleflightCoordinator() *SingleflightCoordinator {
	return &SingleflightCoordinator{
		groups: make(map[CachePolicyKey]*singleflight.Group),
	}
}

func (c *SingleflightCoordinator) group(policyKey CachePolicyKey) *singleflight.Group {
	c.mu.Lock()
	defer c.mu.Unlock()

	if group, ok := c.groups[policyKey]; ok {
		return group
	}
	group := &singleflight.Group{}
	c.groups[policyKey] = group
	return group
}

func (c *SingleflightCoordinator) Do(policyKey CachePolicyKey, key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	return c.group(policyKey).Do(key, fn)
}

var defaultSingleflightCoordinator = NewSingleflightCoordinator()

func sharedSingleflightCoordinator() *SingleflightCoordinator {
	return defaultSingleflightCoordinator
}
