package assessmentmodel

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

type dualStoreV2Stub struct {
	latestCalled bool
	latest       *domain.Snapshot
	latestErr    error
}

func (s *dualStoreV2Stub) UpsertPublished(context.Context, *domain.Snapshot) error {
	return nil
}

func (s *dualStoreV2Stub) GetPublishedByRef(context.Context, port.Ref) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *dualStoreV2Stub) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *dualStoreV2Stub) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*domain.Snapshot, error) {
	s.latestCalled = true
	return s.latest, s.latestErr
}

func (s *dualStoreV2Stub) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	return nil, 0, nil
}

func (s *dualStoreV2Stub) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return nil, nil
}

type dualStoreLegacyStub struct {
	listCalled bool
	snapshots  []*domain.Snapshot
}

func (s *dualStoreLegacyStub) GetPublishedByRef(context.Context, port.Ref) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *dualStoreLegacyStub) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.Snapshot, error) {
	return nil, domain.ErrNotFound
}

func (s *dualStoreLegacyStub) ListPublished(context.Context) ([]*domain.Snapshot, error) {
	s.listCalled = true
	return s.snapshots, nil
}

func TestDualStoreFindPublishedByModelCodeUsesLatestV2Snapshot(t *testing.T) {
	v2 := &dualStoreV2Stub{latest: &domain.Snapshot{
		Definition: domain.Definition{Kind: domain.KindPersonality, Code: "personality_demo", Version: "v4"},
	}}
	legacy := &dualStoreLegacyStub{snapshots: []*domain.Snapshot{{
		Definition: domain.Definition{Kind: domain.KindMBTIMigration, Code: "personality_demo", Version: "v1"},
	}}}
	store := &DualStore{v2: v2, legacy: legacy}

	got, err := store.FindPublishedByModelCode(context.Background(), domain.KindPersonality, "personality_demo")
	if err != nil {
		t.Fatalf("FindPublishedByModelCode: %v", err)
	}
	if !v2.latestCalled {
		t.Fatal("v2 latest lookup was not called")
	}
	if legacy.listCalled {
		t.Fatal("legacy fallback should not be used when v2 latest succeeds")
	}
	if got.Definition.Version != "v4" {
		t.Fatalf("version = %s, want v4", got.Definition.Version)
	}
}
