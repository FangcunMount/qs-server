package cache

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestCachedScaleRepositoryCreateUpdateRemoveWritesAndInvalidatesCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	baseRepo := &scaleMutationRepo{}
	cached := NewCachedScaleRepositoryWithBuilderAndPolicy(
		baseRepo,
		client,
		rediskey.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	).(*CachedScaleRepository)
	domain := newScaleCacheTestScale(t, "S-001")
	key := cached.buildCacheKey("S-001")

	if err := cached.Create(context.Background(), domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !hasRedisKey(t, client, key) {
		t.Fatal("cache key should exist after create")
	}

	if err := cached.Update(context.Background(), domain); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if hasRedisKey(t, client, key) {
		t.Fatal("cache key should be deleted after update")
	}

	if err := cached.Create(context.Background(), domain); err != nil {
		t.Fatalf("Create() second error = %v", err)
	}
	if !hasRedisKey(t, client, key) {
		t.Fatal("cache key should exist before remove")
	}
	if err := cached.Remove(context.Background(), "S-001"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if hasRedisKey(t, client, key) {
		t.Fatal("cache key should be deleted after remove")
	}
}

func TestCachedScaleRepositoryUsesExplicitBuilderNamespace(t *testing.T) {
	repo := NewCachedScaleRepositoryWithBuilderAndPolicy(nil, nil, rediskey.NewBuilderWithNamespace("prod:cache:static"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedScaleRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey("S-001")
	if got != "prod:cache:static:scale:s-001" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}

func newScaleCacheTestScale(t *testing.T, code string) *scale.MedicalScale {
	t.Helper()

	domain, err := scale.NewMedicalScale(meta.NewCode(code), "Test Scale")
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return domain
}

type scaleMutationRepo struct {
	scale.Repository
}

func (r *scaleMutationRepo) Create(context.Context, *scale.MedicalScale) error {
	return nil
}

func (r *scaleMutationRepo) Update(context.Context, *scale.MedicalScale) error {
	return nil
}

func (r *scaleMutationRepo) Remove(context.Context, string) error {
	return nil
}
