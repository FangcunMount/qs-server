package evaluation

import (
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BootstrapInput carries container integration inputs for evaluation module bootstrap.
type BootstrapInput struct {
	MySQLDB                   *gorm.DB
	InputResolver             evaluationinput.Resolver
	ScaleCatalog              evaluationinput.ScaleCatalog
	EventPublisher            event.EventPublisher
	RedisClient               redis.UniversalClient
	CacheBuilder              *keyspace.Builder
	CachePolicies             sharedcache.PolicyProvider
	QueryRedisClient          redis.UniversalClient
	QueryCacheBuilder         *keyspace.Builder
	VersionStore              querycache.VersionTokenStore
	Observer                  *observability.ComponentObserver
	MySQLLimiter              backpressure.Acquirer
	TesteeAccessChecker       evaluationoperator.AccessChecker
	ExecutionPaths            []modelcatalog.ExecutionPath
	RuntimeDescriptorRegistry *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader      rulesetport.PublishedModelReader
	OutboxProfile             appEventing.ProfileBinding
}

// Bootstrap assembles the evaluation module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
