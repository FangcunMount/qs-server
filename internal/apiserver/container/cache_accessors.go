package container

import (
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	redis "github.com/redis/go-redis/v9"
)

func (c *Container) CacheHandle(family cacheplane.Family) *cacheplane.Handle {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Handle(family)
}

func (c *Container) CacheClient(family cacheplane.Family) redis.UniversalClient {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Client(family)
}

func (c *Container) CacheBuilder(family cacheplane.Family) *keyspace.Builder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Builder(family)
}

func (c *Container) CachePolicy(key cachepolicy.CachePolicyKey) cachepolicy.CachePolicy {
	if c == nil || c.cache == nil {
		return cachepolicy.CachePolicy{}
	}
	return c.cache.Policy(key)
}

func (c *Container) cacheObserver() *observability.ComponentObserver {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Observer()
}

func (c *Container) hotsetRecorder() cachetarget.HotsetRecorder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetRecorder()
}

func (c *Container) HotsetInspector() cachetarget.HotsetInspector {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetInspector()
}

func (c *Container) CacheLockManager() locklease.Manager {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.LockManager()
}

func (c *Container) WarmupCoordinator() cachegov.Coordinator {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.WarmupCoordinator()
}

func (c *Container) CacheGovernanceStatusService() cachegov.StatusService {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.StatusService()
}
