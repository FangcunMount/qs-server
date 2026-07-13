package statistics

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// BootstrapInput carries container integration inputs for statistics module bootstrap.
type BootstrapInput struct {
	MySQLDB                *gorm.DB
	RedisClient            redis.UniversalClient
	CacheBuilder           *keyspace.Builder
	AnswerSheetReader      surveyreadmodel.AnswerSheetReader
	AnswerSheetScanSource  statisticsApp.AnswerSheetScanSource
	MongoDB                *mongo.Database
	RepairWindowDays       int
	CachePolicies          sharedcache.PolicyProvider
	SystemStatisticsOpts   statisticsApp.SystemStatisticsOptions
	OverviewGuardOpts      statisticsApp.StatisticsReadGuardOptions
	QuestionnaireGuardOpts statisticsApp.StatisticsReadGuardOptions
	HotsetRecorder         cachetarget.HotsetRecorder
	LockManager            locklease.Manager
	VersionStore           querycache.VersionTokenStore
	Observer               *observability.ComponentObserver
	MySQLLimiter           backpressure.Acquirer
	WarmupCoordinator      statisticsApp.WarmupCoordinator
	StatusService          statisticsApp.GovernanceStatusReader
}

// Bootstrap assembles the statistics module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
