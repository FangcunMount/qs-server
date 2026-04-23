package container

import (
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	redis "github.com/redis/go-redis/v9"
)

func (c *Container) CacheHandle(family redisplane.Family) *redisplane.Handle {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Handle(family)
}

func (c *Container) CacheClient(family redisplane.Family) redis.UniversalClient {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Client(family)
}

func (c *Container) CacheBuilder(family redisplane.Family) *rediskey.Builder {
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

func (c *Container) cacheObserver() *cacheinfra.Observer {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.Observer()
}

func (c *Container) hotsetRecorder() cacheinfra.HotsetRecorder {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetRecorder()
}

func (c *Container) HotsetInspector() cacheinfra.HotsetInspector {
	if c == nil || c.cache == nil {
		return nil
	}
	return c.cache.HotsetInspector()
}

func (c *Container) CacheLockManager() *redislock.Manager {
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
