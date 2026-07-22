package compose

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
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

	CacheClient(family redisruntime.Family) redis.UniversalClient
	CacheBuilder(family redisruntime.Family) *keyspace.Builder
	CacheHandle(family redisruntime.Family) *redisruntime.Handle
	CachePolicyProvider() sharedcache.PolicyProvider
	CacheObserver() *observability.ComponentObserver
	HotsetRecorder() cachetarget.HotsetRecorder
	LockManager() locklease.Manager
	LockRunner() locklease.Runner
	WarmupCoordinator() cachegovernance.WarmupCoordinator
	CacheGovernanceStatusService() cachegovernance.StatusReader
	CacheSignalNotifier() cachebootstrap.SignalNotifier

	IdentityService() *iam.IdentityService
	ActorIAMPorts() ActorIAMPorts

	DefaultEvaluationCatalog() (EvaluationCatalog, error)
	PublishedModelCatalog() rulesetport.Catalog
	InterpretationRunLeaseDuration() time.Duration

	SurveyPorts() SurveyPorts
	ActorPorts() ActorPorts
}
