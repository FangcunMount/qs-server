package plan

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BootstrapInput carries container integration inputs for plan module bootstrap.
type BootstrapInput struct {
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

// Bootstrap assembles the plan module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps(in))
}
