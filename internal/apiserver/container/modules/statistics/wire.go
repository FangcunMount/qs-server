package statistics

import (
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
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
	versionStore := cachequery.NewStaticVersionTokenStore(0)
	redisClient := in.RedisClient
	if !in.DisableStatisticsCache {
		redisClient = in.FallbackRedisClient
	}
	if in.DisableStatisticsCache {
		redisClient = nil
	}
	if !in.DisableStatisticsCache {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			in.MetaRedisClient,
			string(cachepolicy.PolicyStatsQuery),
			in.Observer,
		)
		if versionStore == nil {
			versionStore = cachequery.NewStaticVersionTokenStore(0)
		}
	}
	return Bootstrap(BootstrapInput{
		MySQLDB:               in.MySQLDB,
		RedisClient:           redisClient,
		CacheBuilder:          in.CacheBuilder,
		AnswerSheetReader:     in.AnswerSheetReader,
		AnswerSheetScanSource: in.AnswerSheetScanSource,
		MongoDB:               in.MongoDB,
		RepairWindowDays:      in.RepairWindowDays,
		QueryPolicy:           in.QueryPolicy,
		HotsetRecorder:        in.HotsetRecorder,
		LockManager:           in.LockManager,
		VersionStore:          versionStore,
		Observer:              in.Observer,
		MySQLLimiter:          in.MySQLLimiter,
		WarmupCoordinator:     in.WarmupCoordinator,
		StatusService:         in.StatusService,
	})
}
