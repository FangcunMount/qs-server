package cache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestCachedPlanRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedPlanRepositoryWithBuilderAndPolicy(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:object"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedPlanRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(plan.AssessmentPlanID(meta.MustFromUint64(88)))
	if got != "prod:cache:object:plan:info:88" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
