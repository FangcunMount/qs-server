package plan

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/event"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// BootstrapInput carries container integration inputs for plan module bootstrap.
type BootstrapInput struct {
	MySQLDB         *gorm.DB
	EventPublisher  event.EventPublisher
	PublishedModels modelcatalogport.PublishedModelLister
	RedisClient     redis.UniversalClient
	CacheBuilder    *keyspace.Builder
	CachePolicies   sharedcache.PolicyProvider
	EntryBaseURL    string
	Observer        *observability.ComponentObserver
	MySQLLimiter    backpressure.Acquirer
	TesteeAccess    actorAccessApp.TesteeAccessService
}

// Bootstrap assembles the plan module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
