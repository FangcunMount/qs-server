package plan

import (
	"github.com/FangcunMount/component-base/pkg/event"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for plan module installation.
type WireInput struct {
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

// Wire builds and bootstraps the plan module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
