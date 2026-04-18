package cache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

func TestCachedPlanRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedPlanRepositoryWithBuilderAndPolicy(nil, nil, rediskey.NewBuilderWithNamespace("prod:cache:object"), CachePolicy{})
	cached, ok := repo.(*CachedPlanRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(plan.AssessmentPlanID(meta.MustFromUint64(88)))
	if got != "prod:cache:object:plan:info:88" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
