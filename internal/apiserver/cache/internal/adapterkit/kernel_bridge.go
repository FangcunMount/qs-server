package adapterkit

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	redis "github.com/redis/go-redis/v9"
)

func NewCapabilityObserver(policyKey sharedcache.Capability, health cacheobserve.FamilyObserver) sharedcache.Observer {
	return cacheobserve.NewPrometheus(string(cachepolicy.Family(policyKey)), cachepolicy.MetricLabel(policyKey), health)
}

func NewRedisStoreIfAvailable(client redis.UniversalClient) sharedcache.Store {
	if client == nil {
		return nil
	}
	return redisstore.NewStore(client)
}

func newCapabilityCoalescer(policy sharedcache.Policy) loadguard.Coalescer {
	return loadguard.NewCoalescer(policy.SingleflightEnabled(false))
}

func NewVersionTokenStore(client redis.UniversalClient, policyKey sharedcache.Capability, health cacheobserve.FamilyObserver) querycache.VersionTokenStore {
	observer := cacheobserve.NewQueryVersion(cachepolicy.MetricLabel(policyKey), string(redisruntime.FamilyMeta), health)
	return querycache.NewRedisVersionTokenStore(client, observer)
}
