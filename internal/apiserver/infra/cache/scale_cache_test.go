package cache

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
		keyspace.NewBuilderWithNamespace("test-ns"),
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
	repo := NewCachedScaleRepositoryWithBuilderAndPolicy(nil, nil, keyspace.NewBuilderWithNamespace("prod:cache:static"), cachepolicy.CachePolicy{})
	cached, ok := repo.(*CachedScaleRepository)
	if !ok {
		t.Fatalf("unexpected repository type %T", repo)
	}

	got := cached.buildCacheKey("S-001")
	if got != "prod:cache:static:scale:s-001" {
		t.Fatalf("unexpected cache key: %s", got)
	}
}

func TestCachedScaleRepositoryReloadsPublishedScaleCacheWithoutFactors(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	source := newScaleCacheTestScaleWithFactor(t, "S-001")
	baseRepo := &scaleMutationRepo{findByCodeResult: source}
	cached := NewCachedScaleRepositoryWithBuilderAndPolicy(
		baseRepo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	).(*CachedScaleRepository)
	stale := newScaleCacheTestScale(t, "S-001", scale.WithStatus(scale.StatusPublished))
	if err := cached.setCache(context.Background(), "S-001", stale); err != nil {
		t.Fatalf("set stale cache error = %v", err)
	}

	got, err := cached.FindByCode(context.Background(), "S-001")
	if err != nil {
		t.Fatalf("FindByCode() error = %v", err)
	}
	if got.FactorCount() != 1 {
		t.Fatalf("factor count = %d, want 1", got.FactorCount())
	}
	if baseRepo.findByCodeCalls != 1 {
		t.Fatalf("source loads = %d, want 1", baseRepo.findByCodeCalls)
	}

	got, err = cached.FindByCode(context.Background(), "S-001")
	if err != nil {
		t.Fatalf("FindByCode() second error = %v", err)
	}
	if got.FactorCount() != 1 {
		t.Fatalf("second factor count = %d, want 1", got.FactorCount())
	}
	if baseRepo.findByCodeCalls != 1 {
		t.Fatalf("source loads after refreshed cache = %d, want 1", baseRepo.findByCodeCalls)
	}
}

func newScaleCacheTestScale(t *testing.T, code string, opts ...scale.MedicalScaleOption) *scale.MedicalScale {
	t.Helper()

	domain, err := scale.NewMedicalScale(meta.NewCode(code), "Test Scale", opts...)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return domain
}

func newScaleCacheTestScaleWithFactor(t *testing.T, code string) *scale.MedicalScale {
	t.Helper()

	factor, err := scale.NewFactor(
		scale.NewFactorCode("F1"),
		"Factor 1",
		scale.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	return newScaleCacheTestScale(t, code, scale.WithStatus(scale.StatusPublished), scale.WithFactors([]*scale.Factor{factor}))
}

type scaleMutationRepo struct {
	scale.Repository
	findByCodeResult *scale.MedicalScale
	findByCodeCalls  int
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

func (r *scaleMutationRepo) FindByCode(context.Context, string) (*scale.MedicalScale, error) {
	r.findByCodeCalls++
	return r.findByCodeResult, nil
}
