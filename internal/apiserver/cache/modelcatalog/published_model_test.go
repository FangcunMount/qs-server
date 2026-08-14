package modelcatalogcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func publishedModelPolicies(policy sharedcache.Policy) sharedcache.PolicyProvider {
	return sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityModelCatalogPublished, Enabled: true, Policy: policy})
}

type publishedModelStoreStub struct {
	findByQuestionnaireCalls int
	getByRefCalls            int
	findByCodeCalls          int
	findByCodeKind           domain.Kind
	findByCodeCode           string
	findByQuestionnaire      *port.PublishedModel
	getByRef                 *port.PublishedModel
	findByCode               *port.PublishedModel
	upsertErr                error
}

func (s *publishedModelStoreStub) UpsertPublishedModel(context.Context, *port.PublishedModel) error {
	return s.upsertErr
}

func (s *publishedModelStoreStub) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	s.getByRefCalls++
	if s.getByRef != nil {
		return s.getByRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.PublishedModel, error) {
	s.findByQuestionnaireCalls++
	if s.findByQuestionnaire != nil {
		return s.findByQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedModelByCode(_ context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	s.findByCodeCalls++
	s.findByCodeKind = kind
	s.findByCodeCode = code
	if s.findByCode != nil {
		return s.findByCode, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (s *publishedModelStoreStub) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return nil, nil
}

func TestCachedPublishedModelStoreFindPublishedModelByQuestionnaireCachesUntilCatalogVersionChanges(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &port.PublishedModel{
		Kind:                 domain.KindScale,
		Code:                 "scale-001",
		Version:              "1.0.0",
		QuestionnaireCode:    "q-001",
		QuestionnaireVersion: "1.0.0",
	}
	inner := &publishedModelStoreStub{findByQuestionnaire: snapshot}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(cachepolicy.CachePolicy{}),
		nil,
	)

	got, err := cached.FindPublishedModelByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("first FindPublishedModelByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Code != "scale-001" {
		t.Fatalf("first FindPublishedModelByQuestionnaire() = %#v", got)
	}
	if inner.findByQuestionnaireCalls != 1 {
		t.Fatalf("source calls after first read = %d, want 1", inner.findByQuestionnaireCalls)
	}

	got, err = cached.FindPublishedModelByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("second FindPublishedModelByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Code != "scale-001" {
		t.Fatalf("second FindPublishedModelByQuestionnaire() = %#v", got)
	}
	if inner.findByQuestionnaireCalls != 1 {
		t.Fatalf("source calls after second read = %d, want 1", inner.findByQuestionnaireCalls)
	}

	cached.invalidateCatalogListCaches(context.Background())
	got, err = cached.FindPublishedModelByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil || got == nil {
		t.Fatalf("read after catalog invalidation = %#v, %v", got, err)
	}
	if inner.findByQuestionnaireCalls != 2 {
		t.Fatalf("source calls after catalog invalidation = %d, want 2", inner.findByQuestionnaireCalls)
	}
}

func TestCachedPublishedModelStoreCachesImmutableExactVersionButActiveLookupBypassesIt(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	snapshot := &port.PublishedModel{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0", ReleaseStatus: domain.ReleaseStatusActive}
	inner := &activePublishedModelStoreStub{publishedModelStoreStub: publishedModelStoreStub{getByRef: snapshot}, active: snapshot}
	cached := NewCachedPublishedModelStore(inner, client, keyspace.NewBuilderWithNamespace("test-ns"), publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
	ref := port.Ref{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"}

	if _, err := cached.GetPublishedModelByRef(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.GetPublishedModelByRef(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if inner.getByRefCalls != 1 {
		t.Fatalf("exact source calls = %d, want 1", inner.getByRefCalls)
	}
	if _, err := cached.GetActivePublishedModelByRef(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.GetActivePublishedModelByRef(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if inner.activeCalls != 2 {
		t.Fatalf("active source calls = %d, want 2", inner.activeCalls)
	}
}

func TestCachedPublishedModelStoreImmutableExactVersionL1AvoidsRedisDecode(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	snapshot := &port.PublishedModel{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"}
	inner := &publishedModelStoreStub{getByRef: snapshot}
	cached := NewCachedPublishedModelStore(inner, client, keyspace.NewBuilderWithNamespace("test-ns"), publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
	ref := port.Ref{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"}

	first, err := cached.GetPublishedModelByRef(context.Background(), ref)
	if err != nil || first != snapshot {
		t.Fatalf("first GetPublishedModelByRef() = %#v, %v", first, err)
	}
	key := cached.refCacheKey(ref)
	if err := mr.Set(key, "not-json"); err != nil {
		t.Fatal(err)
	}

	second, err := cached.GetPublishedModelByRef(context.Background(), ref)
	if err != nil || second != snapshot {
		t.Fatalf("L1 GetPublishedModelByRef() = %#v, %v", second, err)
	}
	if inner.getByRefCalls != 1 {
		t.Fatalf("source calls = %d, want 1", inner.getByRefCalls)
	}
	hits, misses := cached.exactByRefL1.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("L1 stats = hits %d misses %d, want 1/1", hits, misses)
	}
}

type activePublishedModelStoreStub struct {
	publishedModelStoreStub
	activeCalls int
	active      *port.PublishedModel
}

func (s *activePublishedModelStoreStub) GetActivePublishedModelByRef(context.Context, port.Ref) (*port.PublishedModel, error) {
	s.activeCalls++
	if s.active == nil {
		return nil, domain.ErrNotFound
	}
	return s.active, nil
}

func TestCachedPublishedModelStoreUpsertPublishedModelDelegatesToInner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &port.PublishedModel{
		Kind:                 domain.KindScale,
		Code:                 "scale-001",
		Version:              "1.0.0",
		QuestionnaireCode:    "q-001",
		QuestionnaireVersion: "1.0.0",
	}
	inner := &publishedModelStoreStub{}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(cachepolicy.CachePolicy{}),
		nil,
	)

	if err := cached.UpsertPublishedModel(context.Background(), snapshot); err != nil {
		t.Fatalf("UpsertPublishedModel() error = %v", err)
	}
}

func TestCachedPublishedModelStoreFindPublishedModelByCodeDelegatesToInner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	published := &port.PublishedModel{
		Kind: domain.KindTypology, Code: "mbti", Version: "1.0.0",
	}
	inner := &publishedModelStoreStub{findByCode: published}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(cachepolicy.CachePolicy{}),
		nil,
	)

	got, err := cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti")
	if err != nil {
		t.Fatalf("first FindPublishedModelByCode() error = %v", err)
	}
	if got == nil || got.Code != "mbti" {
		t.Fatalf("first FindPublishedModelByCode() = %#v", got)
	}

	got, err = cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti")
	if err != nil {
		t.Fatalf("second FindPublishedModelByCode() error = %v", err)
	}
	if got == nil || got.Code != "mbti" {
		t.Fatalf("second FindPublishedModelByCode() = %#v", got)
	}
	if inner.findByCodeCalls != 1 {
		t.Fatalf("source calls after second read = %d, want 1", inner.findByCodeCalls)
	}
	key := keyspace.NewBuilderWithNamespace("test-ns").BuildPublishedAssessmentModelLatestByCodeKey("typology", "mbti")
	if !mr.Exists(key) {
		t.Fatalf("latest-by-code cache key %q does not exist", key)
	}
}

func TestCachedPublishedModelStoreListCatalogCacheKeyIncludesAllQueryPredicates(t *testing.T) {
	cached := NewCachedPublishedModelStore(
		&publishedModelStoreStub{},
		nil,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(cachepolicy.CachePolicy{}),
		nil,
	)
	base := port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "adhd", Page: 1, PageSize: 100}
	cases := []struct {
		name   string
		filter port.ListPublishedFilter
	}{
		{name: "code", filter: port.ListPublishedFilter{Code: "model-b", Kind: domain.KindScale, Category: "adhd", Page: 1, PageSize: 100}},
		{name: "kind", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindTypology, Category: "adhd", Page: 1, PageSize: 100}},
		{name: "algorithm", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Algorithm: domain.AlgorithmPersonalityTypology, Category: "adhd", Page: 1, PageSize: 100}},
		{name: "category", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "slp", Page: 1, PageSize: 100}},
		{name: "keyword", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "adhd", Keyword: "attention", Page: 1, PageSize: 100}},
		{name: "questionnaire", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "adhd", QuestionnaireCode: "q-001", QuestionnaireVersion: "1.0.0", Page: 1, PageSize: 100}},
		{name: "page", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "adhd", Page: 2, PageSize: 100}},
		{name: "page size", filter: port.ListPublishedFilter{Code: "model-a", Kind: domain.KindScale, Category: "adhd", Page: 1, PageSize: 50}},
	}
	baseKey := cached.listCatalogCacheKeyAtVersion(base, 0)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if key := cached.listCatalogCacheKeyAtVersion(tc.filter, 0); key == baseKey {
				t.Fatalf("cache key collision: base=%+v filter=%+v key=%q", base, tc.filter, key)
			}
		})
	}
}

func TestCachedPublishedModelStoreInvalidatePublishedModelClearsExactGetPublishedPage(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})
	cached := NewCachedPublishedModelStore(
		&publishedModelStoreStub{},
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}),
		nil,
	)
	filter := port.ListPublishedFilter{Code: "model-a", Page: 1, PageSize: 100}
	key, err := cached.listCatalogCacheKey(context.Background(), filter)
	if err != nil {
		t.Fatalf("resolve exact GetPublished cache key: %v", err)
	}
	if err := cached.catalogList.Set(context.Background(), key, &publishedModelCatalogListPage{}, sharedcache.Policy{TTL: time.Hour}); err != nil {
		t.Fatalf("populate exact GetPublished cache entry: %v", err)
	}
	if !mr.Exists(key) {
		t.Fatalf("expected exact GetPublished cache entry %q", key)
	}

	cached.invalidatePublishedModel(context.Background(), &port.PublishedModel{Code: "model-a"})
	if mr.Exists(key) {
		t.Fatalf("exact GetPublished cache entry %q was not invalidated", key)
	}
}

func TestCachedPublishedModelStoreInvalidatePublishedModelVersionsOutFilteredCatalogPages(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})
	cached := NewCachedPublishedModelStore(
		&publishedModelStoreStub{},
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}),
		nil,
	)
	ctx := context.Background()
	filter := port.ListPublishedFilter{Kind: domain.KindScale, Category: "emt", Page: 1, PageSize: 20}
	oldKey, err := cached.listCatalogCacheKey(ctx, filter)
	if err != nil {
		t.Fatalf("resolve filtered catalog cache key: %v", err)
	}
	if err := cached.catalogList.Set(ctx, oldKey, &publishedModelCatalogListPage{}, sharedcache.Policy{TTL: time.Hour}); err != nil {
		t.Fatalf("populate filtered catalog cache entry: %v", err)
	}

	cached.invalidatePublishedModel(ctx, &port.PublishedModel{Kind: domain.KindScale, Code: "scale-001"})

	newKey, err := cached.listCatalogCacheKey(ctx, filter)
	if err != nil {
		t.Fatalf("resolve filtered catalog cache key after invalidation: %v", err)
	}
	if newKey == oldKey {
		t.Fatalf("filtered catalog cache key was not versioned: %q", newKey)
	}
	if mr.Exists(newKey) {
		t.Fatalf("new filtered catalog cache entry %q must not reuse stale data", newKey)
	}
}

func TestCachedPublishedModelStoreInvalidatePublishedModelClearsModelByCodeCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &port.PublishedModel{
		Kind: domain.KindTypology, Code: "mbti", Version: "1.0.0",
	}
	inner := &publishedModelStoreStub{findByCode: snapshot}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		publishedModelPolicies(cachepolicy.CachePolicy{}),
		nil,
	)

	if _, err := cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti"); err != nil {
		t.Fatalf("FindPublishedModelByCode error = %v", err)
	}
	cached.invalidatePublishedModel(context.Background(), snapshot)
	if _, err := cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti"); err != nil {
		t.Fatalf("FindPublishedModelByCode after invalidation error = %v", err)
	}
	if inner.findByCodeCalls != 2 {
		t.Fatalf("source calls after invalidation = %d, want 2", inner.findByCodeCalls)
	}
}

func TestCachedPublishedModelStoreWarmByCodeSynchronouslyPublishesVisibleEntry(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	inner := &publishedModelStoreStub{findByCode: &port.PublishedModel{Kind: domain.KindScale, Code: "3adyDE", Version: "1"}}
	cached := NewCachedPublishedModelStore(inner, client, keyspace.NewBuilderWithNamespace("static"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)

	if err := cached.WarmByCode(context.Background(), cachetarget.WarmupKindStaticScale, "3adyDE"); err != nil {
		t.Fatalf("WarmByCode() error = %v", err)
	}
	key := "static:assessment_model:published:latest:scale:3adyde"
	if !mr.Exists(key) {
		t.Fatalf("warmup returned ok before %q existed", key)
	}
	if inner.findByCodeCalls != 1 {
		t.Fatalf("source calls = %d, want 1", inner.findByCodeCalls)
	}
	if inner.findByCodeKind != domain.KindScale || inner.findByCodeCode != "3adyDE" {
		t.Fatalf("source lookup = (%q, %q), want (%q, %q)", inner.findByCodeKind, inner.findByCodeCode, domain.KindScale, "3adyDE")
	}
}

func TestCachedPublishedModelStoreWarmByCodeSkipsWhenUnavailable(t *testing.T) {
	cached := NewCachedPublishedModelStore(&publishedModelStoreStub{}, nil, keyspace.NewBuilderWithNamespace("static"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
	if err := cached.WarmByCode(context.Background(), cachetarget.WarmupKindStaticScale, "SDS"); !errors.Is(err, cachetarget.ErrWarmupSkipped) {
		t.Fatalf("WarmByCode() error = %v, want ErrWarmupSkipped", err)
	}
}
