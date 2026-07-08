package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// DualStore writes and reads v2 published_assessment_models only.
type DualStore struct {
	v2 dualStoreV2Repository
}

type dualStoreV2Repository interface {
	UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error
	GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error)
	FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error)
	ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error)
	ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error)
}

var (
	_ port.PublishedReader          = (*DualStore)(nil)
	_ port.PublishedLister          = (*DualStore)(nil)
	_ port.PublishedModelReader     = (*DualStore)(nil)
	_ port.PublishedModelLister     = (*DualStore)(nil)
	_ port.PublishedWriter          = (*DualStore)(nil)
	_ port.PublishedAlgorithmLister = (*DualStore)(nil)
)

func NewDualStore(v2 *mongomodelcatalog.Repository) *DualStore {
	return &DualStore{v2: v2}
}

func (s *DualStore) UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error {
	if s == nil || s.v2 == nil {
		return domain.ErrNotFound
	}
	return s.v2.UpsertPublished(ctx, snapshot)
}

func (s *DualStore) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.GetPublishedByRef(ctx, ref)
}

func (s *DualStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.GetPublishedByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s *DualStore) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (s *DualStore) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s *DualStore) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (s *DualStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	snapshot, err := s.FindPublishedByModelCode(ctx, kind, code)
	if err != nil {
		return nil, err
	}
	return domain.PublishedFromLegacy(snapshot), nil
}

func (s *DualStore) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	if s == nil || s.v2 == nil {
		return nil, 0, domain.ErrNotFound
	}
	return s.v2.ListPublished(ctx, filter)
}

func (s *DualStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	snapshots, total, err := s.ListPublished(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.PublishedModelSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		out = append(out, domain.PublishedFromLegacy(snapshot))
	}
	return out, total, nil
}

func (s *DualStore) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.ListPublishedAlgorithms(ctx)
}
