package ruleset

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
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

// NewDefaultStaticCatalog 从内置 SBTI/MBTI 与可选量表 repo 回退构建静态规则目录。
func NewDefaultStaticCatalog(scaleSource ScaleBindingSource) (port.RuleSetCatalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets, scaleSource), nil
}

// NewCatalog 优先读 published_assessment_models，未命中时回退 evaluation_rule_sets / 静态 seed。
func NewCatalog(
	db *mongo.Database,
	scaleSource ScaleBindingSource,
	mongoOpts mongoBase.BaseRepositoryOptions,
	cacheCfg PublishedModelCacheConfig,
) (port.RuleSetCatalog, error) {
	static, err := NewDefaultStaticCatalog(scaleSource)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return static, nil
	}
	v2 := mongoassessmentmodel.NewRepository(db, mongoOpts)
	legacy := mongoruleset.NewRepository(db, mongoOpts)
	dual := aminfra.NewDualStore(v2, legacy)
	var store publishedStore = dual
	if cacheCfg.enabled() {
		store = cache.NewCachedPublishedModelStore(dual, cacheCfg.Redis, cacheCfg.Builder, cacheCfg.Policy, cacheCfg.Observer)
	}
	return NewLayeredCatalog(store, static), nil
}
