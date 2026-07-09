package modelcatalog

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type publishedStoreV2Stub struct {
	latestCalled bool
	latest       *port.PublishedModel
	latestErr    error
	list         []*port.PublishedModel
	total        int64
}

func (s *publishedStoreV2Stub) UpsertPublishedModel(context.Context, *port.PublishedModel) error {
	return nil
}

func (s *publishedStoreV2Stub) GetPublishedModelByRef(context.Context, port.Ref) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (s *publishedStoreV2Stub) FindLatestPublishedModelByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	s.latestCalled = true
	return s.latest, s.latestErr
}

func (s *publishedStoreV2Stub) ListPublishedModels(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return s.list, s.total, nil
}

func (s *publishedStoreV2Stub) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return nil, nil
}

func TestPublishedStoreFindPublishedModelByCodeUsesLatestV2Snapshot(t *testing.T) {
	v2 := &publishedStoreV2Stub{latest: &port.PublishedModel{
		Kind: domain.KindTypology, Code: "personality_demo", Version: "v4",
	}}
	store := &PublishedStore{v2: v2}

	got, err := store.FindPublishedModelByCode(context.Background(), domain.KindTypology, "personality_demo")
	if err != nil {
		t.Fatalf("FindPublishedModelByCode: %v", err)
	}
	if !v2.latestCalled {
		t.Fatal("v2 latest lookup was not called")
	}
	if got.Version != "v4" {
		t.Fatalf("version = %s, want v4", got.Version)
	}
}

func TestPublishedStorePublishedModelListerReturnsV2Snapshots(t *testing.T) {
	v2 := &publishedStoreV2Stub{
		latest: &port.PublishedModel{
			Kind: domain.KindTypology, Code: "personality_demo", Version: "v4",
		},
		list: []*port.PublishedModel{{
			Kind: domain.KindTypology, Code: "personality_demo", Version: "v4",
		}},
		total: 1,
	}
	store := &PublishedStore{v2: v2}

	byCode, err := store.FindPublishedModelByCode(context.Background(), domain.KindTypology, "personality_demo")
	if err != nil {
		t.Fatalf("FindPublishedModelByCode: %v", err)
	}
	if byCode.Code != "personality_demo" || byCode.Version != "v4" {
		t.Fatalf("published model = %#v", byCode)
	}
	list, total, err := store.ListPublishedModels(context.Background(), port.ListPublishedFilter{Kind: domain.KindTypology})
	if err != nil {
		t.Fatalf("ListPublishedModels: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("list total=%d len=%d, want 1/1", total, len(list))
	}
	if list[0].Code != "personality_demo" || list[0].Version != "v4" {
		t.Fatalf("list model = %#v", list[0])
	}
}

func TestPublishedStoreReturnsNotFoundWhenV2Misses(t *testing.T) {
	store := &PublishedStore{v2: &publishedStoreV2Stub{}}
	_, err := store.GetPublishedModelByRef(context.Background(), port.Ref{
		Kind: domain.KindTypology, Code: "missing", Version: "1.0.0",
	})
	if err == nil || !domain.IsNotFound(err) {
		t.Fatalf("GetPublishedModelByRef() err = %v, want not found", err)
	}
}
