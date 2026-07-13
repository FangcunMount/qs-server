package compose

import (
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Host is the shared composition-root seam modules use to install themselves.
// Module-specific setters and infra accessors live on per-module InstallHost interfaces.
type Host interface {
	RegisterModule(name string, module modules.Module)
	Printf(format string, args ...any)

	MySQLDB() *gorm.DB
	MongoDB() *mongo.Database
	RedisCache() redis.UniversalClient
	EventPublisher() event.EventPublisher
	TopicResolver() eventcatalog.TopicResolver
	MySQLLimiter() backpressure.Acquirer
	MongoLimiter() backpressure.Acquirer

	OutboxRelayMongoBatchSize() int
	OutboxRelayMongoPublishWorkers() int
	OutboxRelayMongoImmediateMaxConcurrent() int
	OutboxRelayAssessmentBatchSize() int
	OutboxRelayAssessmentPublishWorkers() int
	OutboxRelayAssessmentImmediateMaxConcurrent() int
	PlanEntryBaseURL() string
	StatisticsRepairWindowDays() int
	ReportStatusConfig() reportstatus.Config
	DisableEvaluationCache() bool
	DisableStatisticsCache() bool
	StatisticsSystemOptions() statisticsApp.SystemStatisticsOptions
	StatisticsOverviewGuardOptions() statisticsApp.StatisticsReadGuardOptions
	StatisticsQuestionnaireGuardOptions() statisticsApp.StatisticsReadGuardOptions

	CacheClient(family redisruntime.Family) redis.UniversalClient
	CacheBuilder(family redisruntime.Family) *keyspace.Builder
	CacheHandle(family redisruntime.Family) *redisruntime.Handle
	CachePolicy(key cachepolicy.CachePolicyKey) cachepolicy.CachePolicy
	CacheObserver() *observability.ComponentObserver
	HotsetRecorder() cachetarget.HotsetRecorder
	CacheLockManager() locklease.Manager
	WarmupCoordinator() cachegov.Coordinator
	CacheGovernanceStatusService() cachegov.StatusService
	CacheSignalNotifier() *cachesignal.Notifier

	IdentityService() *iam.IdentityService
	ActorIAMPorts() ActorIAMPorts

	DefaultEvaluationCatalog() (EvaluationCatalog, error)
	PublishedModelCatalog() rulesetport.Catalog
	SetPublishedModelCatalog(catalog rulesetport.Catalog)

	SurveyPorts() SurveyPorts
	ActorPorts() ActorPorts
}
