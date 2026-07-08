package ruleset

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
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

// NewDefaultStaticCatalog 从内置 SBTI/MBTI 与可选量表 repo 构建静态规则目录。
// 仅供 oneoff seed/backfill 与测试使用，生产 composition root 禁止引用。
func NewDefaultStaticCatalog(scaleSource ScaleBindingSource) (port.Catalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets, scaleSource), nil
}
