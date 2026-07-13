package evaluation

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BootstrapInput carries container integration inputs for evaluation module bootstrap.
type BootstrapInput struct {
	MySQLDB                                     *gorm.DB
	InputResolver                               evaluationinput.Resolver
	ScaleCatalog                                evaluationinput.ScaleCatalog
	EventPublisher                              event.EventPublisher
	RedisClient                                 redis.UniversalClient
	CacheBuilder                                *keyspace.Builder
	AssessmentPolicy                            cachepolicy.CachePolicy
	QueryRedisClient                            redis.UniversalClient
	QueryCacheBuilder                           *keyspace.Builder
	AssessmentListPolicy                        cachepolicy.CachePolicy
	VersionStore                                querycache.VersionTokenStore
	Observer                                    *observability.ComponentObserver
	TopicResolver                               eventcatalog.TopicResolver
	MySQLLimiter                                backpressure.Acquirer
	AssessmentOutboxRelayBatchSize              int
	AssessmentOutboxRelayPublishWorkers         int
	AssessmentOutboxRelayImmediateMaxConcurrent int
	TesteeAccessChecker                         evaluationoperator.AccessChecker
	OpsHandle                                   *redisruntime.Handle
	ExecutionPaths                              []modelcatalog.ExecutionPath
	RuntimeDescriptorRegistry                   *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader                        rulesetport.PublishedModelReader
}

// Bootstrap assembles the evaluation module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
