package plancache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
)

func TestCachedPlanRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedPlanRepositoryWithBuilderAndProvider(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:object"), sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityPlanDetail}))
	cached, ok := repo.(*CachedPlanRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(plan.AssessmentPlanID(meta.MustFromUint64(88)))
	if got != "prod:cache:object:plan:info:88" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
