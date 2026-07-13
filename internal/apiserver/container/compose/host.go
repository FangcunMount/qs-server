package compose

import (
	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
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
	EventProfile(eventcatalog.OutboxProfile) appEventing.ProfileBinding
	MySQLLimiter() backpressure.Acquirer
	MongoLimiter() backpressure.Acquirer

	PlanEntryBaseURL() string
	StatisticsRepairWindowDays() int
	ReportStatusConfig() reportstatus.Config
	StatisticsSystemOptions() statisticsApp.SystemStatisticsOptions
	StatisticsOverviewGuardOptions() statisticsApp.StatisticsReadGuardOptions
	StatisticsQuestionnaireGuardOptions() statisticsApp.StatisticsReadGuardOptions

	CacheClient(family redisruntime.Family) redis.UniversalClient
	CacheBuilder(family redisruntime.Family) *keyspace.Builder
	CacheHandle(family redisruntime.Family) *redisruntime.Handle
	CachePolicyProvider() sharedcache.PolicyProvider
	CacheObserver() *observability.ComponentObserver
	HotsetRecorder() cachetarget.HotsetRecorder
	CacheLockManager() locklease.Manager
	WarmupCoordinator() statisticsApp.WarmupCoordinator
	CacheGovernanceStatusService() statisticsApp.GovernanceStatusReader
	CacheSignalNotifier() cachebootstrap.SignalNotifier

	IdentityService() *iam.IdentityService
	ActorIAMPorts() ActorIAMPorts

	DefaultEvaluationCatalog() (EvaluationCatalog, error)
	PublishedModelCatalog() rulesetport.Catalog

	SurveyPorts() SurveyPorts
	ActorPorts() ActorPorts
}
