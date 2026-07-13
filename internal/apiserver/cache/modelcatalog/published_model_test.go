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

func (s *publishedModelStoreStub) FindPublishedModelByCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	s.findByCodeCalls++
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

func TestCachedPublishedModelStoreFindPublishedModelByQuestionnaireDelegatesToInner(t *testing.T) {
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
	if inner.findByQuestionnaireCalls != 2 {
		t.Fatalf("source calls after second read = %d, want 2", inner.findByQuestionnaireCalls)
	}
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
	inner := &publishedModelStoreStub{findByCode: &port.PublishedModel{Kind: domain.KindScale, Code: "SDS", Version: "1"}}
	cached := NewCachedPublishedModelStore(inner, client, keyspace.NewBuilderWithNamespace("static"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)

	if err := cached.WarmByCode(context.Background(), cachetarget.WarmupKindStaticScale, "SDS"); err != nil {
		t.Fatalf("WarmByCode() error = %v", err)
	}
	key := "static:assessment_model:published:latest:scale:sds"
	if !mr.Exists(key) {
		t.Fatalf("warmup returned ok before %q existed", key)
	}
	if inner.findByCodeCalls != 1 {
		t.Fatalf("source calls = %d, want 1", inner.findByCodeCalls)
	}
}

func TestCachedPublishedModelStoreWarmByCodeSkipsWhenUnavailable(t *testing.T) {
	cached := NewCachedPublishedModelStore(&publishedModelStoreStub{}, nil, keyspace.NewBuilderWithNamespace("static"),
		publishedModelPolicies(sharedcache.Policy{TTL: time.Hour}), nil)
	if err := cached.WarmByCode(context.Background(), cachetarget.WarmupKindStaticScale, "SDS"); !errors.Is(err, cachetarget.ErrWarmupSkipped) {
		t.Fatalf("WarmByCode() error = %v, want ErrWarmupSkipped", err)
	}
}
