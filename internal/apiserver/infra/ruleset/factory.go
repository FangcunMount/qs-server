package ruleset

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// PublishedModelCacheConfig wires Redis cache for published_assessment_models hot reads.
type PublishedModelCacheConfig struct {
	Redis    redis.UniversalClient
	Builder  *keyspace.Builder
	Policy   cachepolicy.CachePolicy
	Observer *observability.ComponentObserver
}

func (c PublishedModelCacheConfig) enabled() bool {
	return c.Redis != nil && c.Builder != nil
}

// NewDefaultStaticCatalog builds embedded typology fixtures for tests and
// one-off tooling. Production composition always reads published models.
func NewDefaultStaticCatalog() (port.Catalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets), nil
}
