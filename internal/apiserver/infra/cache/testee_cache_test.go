package cache

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

func TestCachedTesteeRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedTesteeRepositoryWithBuilderAndPolicy(nil, nil, rediskey.NewBuilderWithNamespace("prod:cache:object"), CachePolicy{})
	cached, ok := repo.(*CachedTesteeRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(testee.ID(meta.MustFromUint64(7)))
	if got != "prod:cache:object:testee:info:7" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}
