package plan

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for plan module installation.
type WireInput struct {
	MySQLDB        *gorm.DB
	EventPublisher event.EventPublisher
	ScaleRepo      scaledefinition.Repository
	RedisClient    redis.UniversalClient
	CacheBuilder   *keyspace.Builder
	PlanPolicy     cachepolicy.CachePolicy
	EntryBaseURL   string
	Observer       *observability.ComponentObserver
	MySQLLimiter   backpressure.Acquirer
	TesteeAccess   actorAccessApp.TesteeAccessService
}

// Wire builds and bootstraps the plan module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput(in))
}
