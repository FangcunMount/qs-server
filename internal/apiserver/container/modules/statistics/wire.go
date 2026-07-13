package statistics

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	cacheadapter "github.com/FangcunMount/qs-server/internal/apiserver/cache/adapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for statistics module installation.
type WireInput struct {
	MySQLDB                *gorm.DB
	RedisClient            redis.UniversalClient
	FallbackRedisClient    redis.UniversalClient
	CacheBuilder           *keyspace.Builder
	AnswerSheetReader      surveyreadmodel.AnswerSheetReader
	AnswerSheetScanSource  statisticsApp.AnswerSheetScanSource
	MongoDB                *mongo.Database
	RepairWindowDays       int
	QueryPolicy            cachepolicy.CachePolicy
	SystemStatisticsOpts   statisticsApp.SystemStatisticsOptions
	OverviewGuardOpts      statisticsApp.StatisticsReadGuardOptions
	QuestionnaireGuardOpts statisticsApp.StatisticsReadGuardOptions
	HotsetRecorder         cachetarget.HotsetRecorder
	LockManager            locklease.Manager
	Observer               *observability.ComponentObserver
	MySQLLimiter           backpressure.Acquirer
	WarmupCoordinator      cachegov.Coordinator
	StatusService          cachegov.StatusService
	DisableStatisticsCache bool
	MetaRedisClient        redis.UniversalClient
}

// Wire builds and bootstraps the statistics module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	versionStore := querycache.NewStaticVersionTokenStore(0)
	redisClient := in.RedisClient
	if !in.DisableStatisticsCache {
		redisClient = in.FallbackRedisClient
	}
	if in.DisableStatisticsCache {
		redisClient = nil
	}
	if !in.DisableStatisticsCache {
		versionStore = cacheadapter.NewVersionTokenStore(in.MetaRedisClient, cachepolicy.PolicyStatsQuery, in.Observer)
		if versionStore == nil {
			versionStore = querycache.NewStaticVersionTokenStore(0)
		}
	}
	return Bootstrap(BootstrapInput{
		MySQLDB:                in.MySQLDB,
		RedisClient:            redisClient,
		CacheBuilder:           in.CacheBuilder,
		AnswerSheetReader:      in.AnswerSheetReader,
		AnswerSheetScanSource:  in.AnswerSheetScanSource,
		MongoDB:                in.MongoDB,
		RepairWindowDays:       in.RepairWindowDays,
		QueryPolicy:            in.QueryPolicy,
		SystemStatisticsOpts:   in.SystemStatisticsOpts,
		OverviewGuardOpts:      in.OverviewGuardOpts,
		QuestionnaireGuardOpts: in.QuestionnaireGuardOpts,
		HotsetRecorder:         in.HotsetRecorder,
		LockManager:            in.LockManager,
		VersionStore:           versionStore,
		Observer:               in.Observer,
		MySQLLimiter:           in.MySQLLimiter,
		WarmupCoordinator:      in.WarmupCoordinator,
		StatusService:          in.StatusService,
	})
}
