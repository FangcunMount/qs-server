package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedStore writes and reads v2 published_assessment_models only.
type PublishedStore struct {
	v2 publishedStoreV2Repository
}

type publishedStoreV2Repository interface {
	UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error
	UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error
	GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error)
	GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error)
	FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error)
	FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error)
	FindLatestPublishedModelByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error)
	ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error)
	ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error)
	ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error)
}

var (
	_ port.PublishedReader          = (*PublishedStore)(nil)
	_ port.PublishedLister          = (*PublishedStore)(nil)
	_ port.PublishedModelReader     = (*PublishedStore)(nil)
	_ port.PublishedModelLister     = (*PublishedStore)(nil)
	_ port.PublishedWriter          = (*PublishedStore)(nil)
	_ port.PublishedAlgorithmLister = (*PublishedStore)(nil)
)

func NewPublishedStore(v2 *mongomodelcatalog.Repository) *PublishedStore {
	return &PublishedStore{v2: v2}
}

func (s *PublishedStore) UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error {
	if s == nil || s.v2 == nil {
		return domain.ErrNotFound
	}
	return s.v2.UpsertPublished(ctx, snapshot)
}

func (s *PublishedStore) UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if s == nil || s.v2 == nil {
		return domain.ErrNotFound
	}
	return s.v2.UpsertPublishedModel(ctx, snapshot)
}

func (s *PublishedStore) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.GetPublishedByRef(ctx, ref)
}

func (s *PublishedStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.GetPublishedModelByRef(ctx, ref)
}

func (s *PublishedStore) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (s *PublishedStore) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (s *PublishedStore) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (s *PublishedStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindLatestPublishedModelByModelCode(ctx, kind, code)
}

func (s *PublishedStore) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	if s == nil || s.v2 == nil {
		return nil, 0, domain.ErrNotFound
	}
	return s.v2.ListPublished(ctx, filter)
}

func (s *PublishedStore) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	if s == nil || s.v2 == nil {
		return nil, 0, domain.ErrNotFound
	}
	return s.v2.ListPublishedModels(ctx, filter)
}

func (s *PublishedStore) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.ListPublishedAlgorithms(ctx)
}
