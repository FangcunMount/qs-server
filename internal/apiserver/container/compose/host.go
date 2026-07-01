package compose

import (
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
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
	PlanEntryBaseURL() string
	StatisticsRepairWindowDays() int
	ReportStatusConfig() reportstatus.Config
	DisableEvaluationCache() bool
	DisableStatisticsCache() bool
	StatisticsSystemOptions() statisticsApp.SystemStatisticsOptions

	CacheClient(family cacheplane.Family) redis.UniversalClient
	CacheBuilder(family cacheplane.Family) *keyspace.Builder
	CacheHandle(family cacheplane.Family) *cacheplane.Handle
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
	RuleSetCatalog() rulesetport.RuleSetCatalog
	SetRuleSetCatalog(catalog rulesetport.RuleSetCatalog)

	SurveyPorts() SurveyPorts
	ActorPorts() ActorPorts
	ReportIntegrationPorts() ReportIntegrationPorts
}
