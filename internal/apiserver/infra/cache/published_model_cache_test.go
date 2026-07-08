package cache

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type publishedModelStoreStub struct {
	findByQuestionnaireCalls int
	getByRefCalls            int
	findByCodeCalls          int
	findByQuestionnaire      *domain.PublishedModelSnapshot
	getByRef                 *domain.PublishedModelSnapshot
	findByCode               *domain.PublishedModelSnapshot
	upsertErr                error
}

func (s *publishedModelStoreStub) UpsertPublishedModel(context.Context, *domain.PublishedModelSnapshot) error {
	return s.upsertErr
}

func (s *publishedModelStoreStub) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	s.getByRefCalls++
	if s.getByRef != nil {
		return s.getByRef, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	s.findByQuestionnaireCalls++
	if s.findByQuestionnaire != nil {
		return s.findByQuestionnaire, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) FindPublishedModelByCode(context.Context, domain.Kind, string) (*domain.PublishedModelSnapshot, error) {
	s.findByCodeCalls++
	if s.findByCode != nil {
		return s.findByCode, nil
	}
	return nil, domain.ErrNotFound
}

func (s *publishedModelStoreStub) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
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

	snapshot := &domain.PublishedModelSnapshot{
		Model: domain.ModelDefinition{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"},
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

	got, err := cached.FindPublishedModelByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("first FindPublishedModelByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Model.Code != "scale-001" {
		t.Fatalf("first FindPublishedModelByQuestionnaire() = %#v", got)
	}
	if inner.findByQuestionnaireCalls != 1 {
		t.Fatalf("source calls after first read = %d, want 1", inner.findByQuestionnaireCalls)
	}

	got, err = cached.FindPublishedModelByQuestionnaire(context.Background(), "q-001", "1.0.0")
	if err != nil {
		t.Fatalf("second FindPublishedModelByQuestionnaire() error = %v", err)
	}
	if got == nil || got.Model.Code != "scale-001" {
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

	snapshot := &domain.PublishedModelSnapshot{
		Model: domain.ModelDefinition{Kind: domain.KindScale, Code: "scale-001", Version: "1.0.0"},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "q-001",
			QuestionnaireVersion: "1.0.0",
		},
	}
	inner := &publishedModelStoreStub{}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
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

	published := &domain.PublishedModelSnapshot{
		Model: domain.ModelDefinition{Kind: domain.KindTypology, Code: "mbti", Version: "1.0.0"},
	}
	inner := &publishedModelStoreStub{findByCode: published}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
		nil,
	)

	got, err := cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti")
	if err != nil {
		t.Fatalf("first FindPublishedModelByCode() error = %v", err)
	}
	if got == nil || got.Model.Code != "mbti" {
		t.Fatalf("first FindPublishedModelByCode() = %#v", got)
	}

	got, err = cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti")
	if err != nil {
		t.Fatalf("second FindPublishedModelByCode() error = %v", err)
	}
	if got == nil || got.Model.Code != "mbti" {
		t.Fatalf("second FindPublishedModelByCode() = %#v", got)
	}
	if inner.findByCodeCalls != 2 {
		t.Fatalf("source calls after second read = %d, want 2", inner.findByCodeCalls)
	}
}

func TestCachedPublishedModelStoreInvalidatePublishedSnapshotClearsModelByCodeCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	snapshot := &domain.PublishedModelSnapshot{
		Model: domain.ModelDefinition{Kind: domain.KindTypology, Code: "mbti", Version: "1.0.0"},
	}
	inner := &publishedModelStoreStub{findByCode: snapshot}
	cached := NewCachedPublishedModelStore(
		inner,
		client,
		keyspace.NewBuilderWithNamespace("test-ns"),
		cachepolicy.CachePolicy{},
		nil,
	)

	if _, err := cached.FindPublishedModelByCode(context.Background(), domain.KindTypology, "mbti"); err != nil {
		t.Fatalf("FindPublishedModelByCode error = %v", err)
	}
	cached.invalidatePublishedSnapshot(context.Background(), snapshot)
}
