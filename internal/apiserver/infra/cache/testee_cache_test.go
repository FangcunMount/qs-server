package cache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestCachedTesteeRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedTesteeRepositoryWithBuilderAndPolicy(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:object"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedTesteeRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey(testee.ID(meta.MustFromUint64(7)))
	if got != "prod:cache:object:testee:info:7" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}

func TestCachedTesteeRepositoryCachesNegativeResult(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	repo := &testeeNegativeRepo{}
	cached := NewCachedTesteeRepositoryWithBuilderAndPolicy(
		repo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{
			Negative:    cachepolicy.PolicySwitchEnabled,
			NegativeTTL: time.Minute,
		},
	).(*CachedTesteeRepository)

	ctx := context.Background()
	id := testee.ID(meta.MustFromUint64(99))
	got, err := cached.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got != nil {
		t.Fatalf("FindByID() value = %#v, want nil", got)
	}

	key := cached.buildCacheKey(id)
	waitFor(t, func() bool {
		return hasRedisKey(t, client, key)
	})

	got, err = cached.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() cached negative error = %v", err)
	}
	if got != nil {
		t.Fatalf("FindByID() cached negative value = %#v, want nil", got)
	}
	if repo.findByIDCalls != 1 {
		t.Fatalf("FindByID() repo calls = %d, want 1", repo.findByIDCalls)
	}
}

type testeeNegativeRepo struct {
	testee.Repository
	findByIDCalls int
}

func (r *testeeNegativeRepo) FindByID(context.Context, testee.ID) (*testee.Testee, error) {
	r.findByIDCalls++
	return nil, nil
}
