package cache

import (
	"context"
	"testing"
	"time"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition"
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
	versionKey := cached.buildVersionCacheKey("S-001", domain.GetScaleVersion())

	if err := cached.Create(context.Background(), domain); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !hasRedisKey(t, client, key) {
		t.Fatal("cache key should exist after create")
	}
	if !hasRedisKey(t, client, versionKey) {
		t.Fatal("version cache key should exist after create")
	}

	if err := cached.Update(context.Background(), domain); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if hasRedisKey(t, client, key) {
		t.Fatal("cache key should be deleted after update")
	}
	if hasRedisKey(t, client, versionKey) {
		t.Fatal("version cache key should be deleted after update")
	}

	if err := cached.Create(context.Background(), domain); err != nil {
		t.Fatalf("Create() second error = %v", err)
	}
	if !hasRedisKey(t, client, key) {
		t.Fatal("cache key should exist before remove")
	}
	if !hasRedisKey(t, client, versionKey) {
		t.Fatal("version cache key should exist before remove")
	}
	baseRepo.findByCodeResult = domain
	if err := cached.Remove(context.Background(), "S-001"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if hasRedisKey(t, client, key) {
		t.Fatal("cache key should be deleted after remove")
	}
	if hasRedisKey(t, client, versionKey) {
		t.Fatal("version cache key should be deleted after remove")
	}
}

func TestCachedScaleRepositoryUpdateInvalidatesOldAndNewVersionCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	oldDomain := newScaleCacheTestScale(t, "S-001", scaledefinition.WithScaleVersion("1.0.0"))
	newDomain := newScaleCacheTestScale(t, "S-001", scaledefinition.WithScaleVersion("2.0.0"))
	baseRepo := &scaleMutationRepo{findByCodeResult: oldDomain}
	cached := NewCachedScaleRepositoryWithBuilderAndPolicy(
		baseRepo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	).(*CachedScaleRepository)
	codeKey := cached.buildCacheKey("S-001")
	oldVersionKey := cached.buildVersionCacheKey("S-001", "1.0.0")
	newVersionKey := cached.buildVersionCacheKey("S-001", "2.0.0")

	if err := cached.setCache(context.Background(), "S-001", oldDomain); err != nil {
		t.Fatalf("set cache error = %v", err)
	}
	if err := cached.setVersionCache(context.Background(), "S-001", "1.0.0", oldDomain); err != nil {
		t.Fatalf("set old version cache error = %v", err)
	}
	if err := cached.setVersionCache(context.Background(), "S-001", "2.0.0", newDomain); err != nil {
		t.Fatalf("set new version cache error = %v", err)
	}

	if err := cached.Update(context.Background(), newDomain); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for _, key := range []string{codeKey, oldVersionKey, newVersionKey} {
		if hasRedisKey(t, client, key) {
			t.Fatalf("cache key %s should be deleted after update", key)
		}
	}
	if baseRepo.findByCodeCalls != 1 {
		t.Fatalf("FindByCode calls = %d, want 1 to discover old version", baseRepo.findByCodeCalls)
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

func TestCachedScaleRepositoryFindPublishedByCodeUsesCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	source := newScaleCacheTestScaleWithFactor(t, "S-001")
	baseRepo := &scaleMutationRepo{findPublishedByCodeResult: source}
	cached := NewCachedScaleRepositoryWithBuilderAndPolicy(
		baseRepo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	).(*CachedScaleRepository)
	publishedKey := cached.buildPublishedScaleCacheKey("S-001")

	got, err := cached.FindPublishedByCode(context.Background(), "S-001")
	if err != nil {
		t.Fatalf("FindPublishedByCode() first error = %v", err)
	}
	if got.FactorCount() != 1 {
		t.Fatalf("factor count = %d, want 1", got.FactorCount())
	}
	if baseRepo.findPublishedByCodeCalls != 1 {
		t.Fatalf("source loads = %d, want 1", baseRepo.findPublishedByCodeCalls)
	}
	waitForRedisKey(t, client, publishedKey)

	got, err = cached.FindPublishedByCode(context.Background(), "S-001")
	if err != nil {
		t.Fatalf("FindPublishedByCode() second error = %v", err)
	}
	if got.FactorCount() != 1 {
		t.Fatalf("second factor count = %d, want 1", got.FactorCount())
	}
	if baseRepo.findPublishedByCodeCalls != 1 {
		t.Fatalf("source loads after cache hit = %d, want 1", baseRepo.findPublishedByCodeCalls)
	}
}

func TestCachedScaleRepositoryFindPublishedByQuestionnaireCodeUsesCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	source := newScaleCacheTestScaleWithFactor(t, "S-001")
	baseRepo := &scaleMutationRepo{findPublishedByQuestionnaireCodeResult: source}
	cached := NewCachedScaleRepositoryWithBuilderAndPolicy(
		baseRepo,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
	).(*CachedScaleRepository)
	questionnaireKey := cached.buildPublishedScaleByQuestionnaireCacheKey("Q-001")

	if _, err := cached.FindPublishedByQuestionnaireCode(context.Background(), "Q-001"); err != nil {
		t.Fatalf("FindPublishedByQuestionnaireCode() first error = %v", err)
	}
	if baseRepo.findPublishedByQuestionnaireCodeCalls != 1 {
		t.Fatalf("source loads = %d, want 1", baseRepo.findPublishedByQuestionnaireCodeCalls)
	}
	waitForRedisKey(t, client, questionnaireKey)

	if _, err := cached.FindPublishedByQuestionnaireCode(context.Background(), "Q-001"); err != nil {
		t.Fatalf("FindPublishedByQuestionnaireCode() second error = %v", err)
	}
	if baseRepo.findPublishedByQuestionnaireCodeCalls != 1 {
		t.Fatalf("source loads after cache hit = %d, want 1", baseRepo.findPublishedByQuestionnaireCodeCalls)
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
	stale := newScaleCacheTestScale(t, "S-001", scaledefinition.WithStatus(scaledefinition.StatusPublished))
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

func newScaleCacheTestScale(t *testing.T, code string, opts ...scaledefinition.MedicalScaleOption) *scaledefinition.MedicalScale {
	t.Helper()

	domain, err := scaledefinition.NewMedicalScale(meta.NewCode(code), "Test Scale", opts...)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return domain
}

func newScaleCacheTestScaleWithFactor(t *testing.T, code string) *scaledefinition.MedicalScale {
	t.Helper()

	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("F1"),
		"Factor 1",
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	return newScaleCacheTestScale(t, code, scaledefinition.WithStatus(scaledefinition.StatusPublished), scaledefinition.WithFactors([]*scaledefinition.Factor{factor}))
}

type scaleMutationRepo struct {
	scaledefinition.Repository
	findByCodeResult                       *scaledefinition.MedicalScale
	findByCodeCalls                        int
	findPublishedByCodeResult              *scaledefinition.MedicalScale
	findPublishedByCodeCalls               int
	findPublishedByQuestionnaireCodeResult *scaledefinition.MedicalScale
	findPublishedByQuestionnaireCodeCalls  int
}

func (r *scaleMutationRepo) Create(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}

func (r *scaleMutationRepo) Update(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}

func (r *scaleMutationRepo) Remove(context.Context, string) error {
	return nil
}

func (r *scaleMutationRepo) FindByCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	r.findByCodeCalls++
	return r.findByCodeResult, nil
}

func (r *scaleMutationRepo) FindPublishedByCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	r.findPublishedByCodeCalls++
	return r.findPublishedByCodeResult, nil
}

func (r *scaleMutationRepo) FindPublishedByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	r.findPublishedByQuestionnaireCodeCalls++
	return r.findPublishedByQuestionnaireCodeResult, nil
}

func waitForRedisKey(t *testing.T, client redis.UniversalClient, key string) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if hasRedisKey(t, client, key) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("redis key %s was not populated before deadline", key)
}
