package statistics

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/cache/statistics"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for statistics module installation.
type WireInput struct {
	MySQLDB               *gorm.DB
	FallbackRedisClient   redis.UniversalClient
	CacheBuilder          *keyspace.Builder
	AnswerSheetScanSource statisticsApp.AnswerSheetScanSource
	MongoDB               *mongo.Database
	RepairWindowDays      int
	CachePolicies         sharedcache.PolicyProvider
	OverviewGuardOpts     statisticsApp.StatisticsReadGuardOptions
	HotsetRecorder        cachetarget.HotsetRecorder
	LockManager           locklease.Manager
	Observer              *observability.ComponentObserver
	MySQLLimiter          backpressure.Acquirer
	WarmupCoordinator     statisticsApp.WarmupCoordinator
	StatusService         statisticsApp.GovernanceStatusReader
	MetaRedisClient       redis.UniversalClient
}

// Wire builds and bootstraps the statistics module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	versionStore := querycache.NewStaticVersionTokenStore(0)
	if in.FallbackRedisClient != nil {
		versionStore = statisticsCache.NewVersionTokenStore(in.MetaRedisClient, in.Observer)
		if versionStore == nil {
			versionStore = querycache.NewStaticVersionTokenStore(0)
		}
	}
	return Bootstrap(BootstrapInput{
		MySQLDB:               in.MySQLDB,
		RedisClient:           in.FallbackRedisClient,
		CacheBuilder:          in.CacheBuilder,
		AnswerSheetScanSource: in.AnswerSheetScanSource,
		MongoDB:               in.MongoDB,
		RepairWindowDays:      in.RepairWindowDays,
		CachePolicies:         in.CachePolicies,
		OverviewGuardOpts:     in.OverviewGuardOpts,
		HotsetRecorder:        in.HotsetRecorder,
		LockManager:           in.LockManager,
		VersionStore:          versionStore,
		Observer:              in.Observer,
		MySQLLimiter:          in.MySQLLimiter,
		WarmupCoordinator:     in.WarmupCoordinator,
		StatusService:         in.StatusService,
	})
}
