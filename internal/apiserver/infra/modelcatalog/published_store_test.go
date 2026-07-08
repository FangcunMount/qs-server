package modelcatalog

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type publishedStoreV2Stub struct {
	latestCalled bool
	latest       *domain.Snapshot
	latestErr    error
	list         []*domain.Snapshot
	total        int64
}

func (s *publishedStoreV2Stub) UpsertPublished(context.Context, *domain.Snapshot) error {
	return nil
}

func (s *publishedStoreV2Stub) UpsertPublishedModel(context.Context, *domain.PublishedModelSnapshot) error {
	return nil
}

func (s *publishedStoreV2Stub) GetPublishedModelByRef(context.Context, port.Ref) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindLatestPublishedModelByModelCode(context.Context, domain.Kind, string) (*domain.PublishedModelSnapshot, error) {
	s.latestCalled = true
	if s.latest != nil {
		return domain.PublishedFromLegacy(s.latest), s.latestErr
	}
	return nil, s.latestErr
}

func (s *publishedStoreV2Stub) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	out := make([]*domain.PublishedModelSnapshot, 0, len(s.list))
	for _, snapshot := range s.list {
		if snapshot == nil {
			continue
		}
		out = append(out, domain.PublishedFromLegacy(snapshot))
	}
	return out, s.total, nil
}

func (s *publishedStoreV2Stub) GetPublishedByRef(context.Context, port.Ref) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*domain.Snapshot, error) {
	s.latestCalled = true
	return s.latest, s.latestErr
}

func (s *publishedStoreV2Stub) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	return s.list, s.total, nil
}

func (s *publishedStoreV2Stub) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return nil, nil
}

func TestPublishedStoreFindPublishedByModelCodeUsesLatestV2Snapshot(t *testing.T) {
	v2 := &publishedStoreV2Stub{latest: &domain.Snapshot{
		Definition: domain.Definition{Kind: domain.KindPersonality, Code: "personality_demo", Version: "v4"},
	}}
	store := &PublishedStore{v2: v2}

	got, err := store.FindPublishedByModelCode(context.Background(), domain.KindPersonality, "personality_demo")
	if err != nil {
		t.Fatalf("FindPublishedByModelCode: %v", err)
	}
	if !v2.latestCalled {
		t.Fatal("v2 latest lookup was not called")
	}
	if got.Definition.Version != "v4" {
		t.Fatalf("version = %s, want v4", got.Definition.Version)
	}
}

func TestPublishedStorePublishedModelListerReturnsV2Snapshots(t *testing.T) {
	v2 := &publishedStoreV2Stub{
		latest: &domain.Snapshot{
			Definition: domain.Definition{Kind: domain.KindPersonality, Code: "personality_demo", Version: "v4"},
		},
		list: []*domain.Snapshot{{
			Definition: domain.Definition{Kind: domain.KindPersonality, Code: "personality_demo", Version: "v4"},
		}},
		total: 1,
	}
	store := &PublishedStore{v2: v2}

	byCode, err := store.FindPublishedModelByCode(context.Background(), domain.KindPersonality, "personality_demo")
	if err != nil {
		t.Fatalf("FindPublishedModelByCode: %v", err)
	}
	if byCode.Model.Code != "personality_demo" || byCode.Model.Version != "v4" {
		t.Fatalf("published model = %#v", byCode.Model)
	}
	list, total, err := store.ListPublishedModels(context.Background(), port.ListPublishedFilter{Kind: domain.KindPersonality})
	if err != nil {
		t.Fatalf("ListPublishedModels: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("list total=%d len=%d, want 1/1", total, len(list))
	}
	if list[0].Model.Code != "personality_demo" || list[0].Model.Version != "v4" {
		t.Fatalf("list model = %#v", list[0].Model)
	}
}

func TestPublishedStoreReturnsNotFoundWhenV2Misses(t *testing.T) {
	store := &PublishedStore{v2: &publishedStoreV2Stub{}}
	_, err := store.GetPublishedByRef(context.Background(), port.Ref{
		Kind: domain.KindPersonality, Code: "missing", Version: "1.0.0",
	})
	if err == nil || !domain.IsNotFound(err) {
		t.Fatalf("GetPublishedByRef() err = %v, want not found", err)
	}
}
