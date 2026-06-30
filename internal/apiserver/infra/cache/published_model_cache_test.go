package cache

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type publishedModelStoreStub struct {
	findByQuestionnaireCalls int
	getByRefCalls            int
	findByQuestionnaire      *domain.Snapshot
	getByRef                 *domain.Snapshot
	upsertErr                error
}

func (s *publishedModelStoreStub) UpsertPublished(context.Context, *domain.Snapshot) error {
	return s.upsertErr
}

func (s *publishedModelStoreStub) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	s.getByRefCalls++
	if s.getByRef != nil {
		return s.getByRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error) {
	s.findByQuestionnaireCalls++
	if s.findByQuestionnaire != nil {
		return s.findByQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.GetPublishedByRef(ctx, ref)
	if err != nil || snapshot == nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s *publishedModelStoreStub) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil || snapshot == nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s *publishedModelStoreStub) FindPublishedByModelCode(context.Context, domain.Kind, string) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedModelByCode(context.Context, domain.Kind, string) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	return nil, 0, nil
}

func (s *publishedModelStoreStub) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	return nil, 0, nil
}

func (s *publishedModelStoreStub) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return nil, nil
}

func TestCachedPublishedModelStoreFindPublishedByQuestionnaireCachesHit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &domain.Snapshot{
		Definition: domain.Definition{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "q-001",
			QuestionnaireVersion: "1.0.0",
		},
	}
	inner := &publishedModelStoreStub{findByQuestionnaire: snapshot}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
		nil,
	)

	got, err := cached.FindPublishedByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("first FindPublishedByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Definition.Code != "scale-001" {
		t.Fatalf("first FindPublishedByQuestionnaire() = %#v", got)
	}
	if inner.findByQuestionnaireCalls != 1 {
		t.Fatalf("source calls after first read = %d, want 1", inner.findByQuestionnaireCalls)
	}
	waitFor(t, func() bool {
		return hasRedisKey(t, client, cached.questionnaireCacheKey("q-001", "1.0.0"))
	})

	got, err = cached.FindPublishedByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("second FindPublishedByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Definition.Code != "scale-001" {
		t.Fatalf("second FindPublishedByQuestionnaire() = %#v", got)
	}
	if inner.findByQuestionnaireCalls != 1 {
		t.Fatalf("source calls after cache hit = %d, want 1", inner.findByQuestionnaireCalls)
	}
}

func TestCachedPublishedModelStoreUpsertPublishedInvalidatesCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &domain.Snapshot{
		Definition: domain.Definition{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "q-001",
			QuestionnaireVersion: "1.0.0",
		},
	}
	inner := &publishedModelStoreStub{findByQuestionnaire: snapshot}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
		nil,
	)
	cacheKey := cached.questionnaireCacheKey("q-001", "1.0.0")

	if _, err := cached.FindPublishedByQuestionnaire(context.Background(), "q-001", "1.0.0"); err != nil {
		t.Fatalf("warm cache error = %v", err)
	}
	waitFor(t, func() bool {
		return hasRedisKey(t, client, cacheKey)
	})

	if err := cached.UpsertPublished(context.Background(), snapshot); err != nil {
		t.Fatalf("UpsertPublished() error = %v", err)
	}
	if hasRedisKey(t, client, cacheKey) {
		t.Fatal("cache key should be deleted after upsert invalidation")
	}
}
