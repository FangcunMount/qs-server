package evaluation

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
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
	VersionStore                                cachequery.VersionTokenStore
	Observer                                    *observability.ComponentObserver
	TopicResolver                               eventcatalog.TopicResolver
	MySQLLimiter                                backpressure.Acquirer
	AssessmentOutboxRelayBatchSize              int
	AssessmentOutboxRelayPublishWorkers         int
	AssessmentOutboxRelayImmediateMaxConcurrent int
	TesteeAccessChecker                         assessment.TesteeAccessChecker
	OpsHandle                                   *cacheplane.Handle
	ModelDescriptors                            []evaldomain.ModelDescriptor
	TypologyRegistry                            evalregistry.TypologyRegistry
	RuntimeDescriptorRegistry                   *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader                        rulesetport.PublishedModelReader
}

// Bootstrap assembles the evaluation module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
