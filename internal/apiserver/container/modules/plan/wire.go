package plan

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for plan module installation.
type WireInput struct {
	MySQLDB                   *gorm.DB
	EventPublisher            event.EventPublisher
	PublishedModels           modelcatalogport.PublishedModelLister
	RedisClient               redis.UniversalClient
	CacheBuilder              *keyspace.Builder
	CachePolicies             sharedcache.PolicyProvider
	EntryBaseURL              string
	Observer                  *observability.ComponentObserver
	MySQLLimiter              backpressure.Acquirer
	TesteeAccess              actorAccessApp.TesteeAccessService
	OpenedOutbox              appEventing.ProfileBinding
	TaskOpenedReminderEnabled bool
	EventTransactionOwner     *mysqlEventTx.Committer
	TransactionRoutes         eventcatalog.TopicResolver
}

// Wire builds and bootstraps the plan module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
