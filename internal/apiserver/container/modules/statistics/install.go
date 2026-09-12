package statistics

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

type InstallHost interface {
	compose.Host
	SetStatisticsModule(*Module)
}

func InstallFrom(host InstallHost, access ...statisticsApp.StatisticsScopeAccess) error {
	var scopeAccess statisticsApp.StatisticsScopeAccess
	if len(access) > 0 {
		scopeAccess = access[0]
	}
	binding := compose.ResolveCacheCapability(host.CachePolicyProvider(), cachepolicy.CapabilityStatisticsQuery)
	queryRedis := host.CacheClient(redisruntime.FamilyQuery)
	if !binding.Enabled {
		queryRedis = nil
	}
	module, err := Wire(Deps{
		ScopeAccess:  scopeAccess,
		MySQLDB:      host.MySQLDB(),
		MongoDB:      host.MongoDB(),
		RedisClient:  queryRedis,
		QueryBuilder: host.CacheBuilder(redisruntime.FamilyQuery),
		LockRunner:   host.LockRunner(),
		MySQLLimiter: host.MySQLLimiter(),
		QueryTTL:     binding.Policy.TTLOr(26 * time.Hour),
	})
	if err != nil {
		return err
	}
	host.SetStatisticsModule(module)
	host.RegisterModule("statistics", module)
	host.Printf("📦 Statistics module initialized\n")
	return nil
}
