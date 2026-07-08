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
	UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error
	GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error)
	FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error)
	FindLatestPublishedModelByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error)
	ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error)
	ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error)
}

var (
	_ port.PublishedModelReader     = (*PublishedStore)(nil)
	_ port.PublishedModelLister     = (*PublishedStore)(nil)
	_ port.PublishedWriter          = (*PublishedStore)(nil)
	_ port.PublishedAlgorithmLister = (*PublishedStore)(nil)
)

func NewPublishedStore(v2 *mongomodelcatalog.Repository) *PublishedStore {
	return &PublishedStore{v2: v2}
}

func (s *PublishedStore) UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if s == nil || s.v2 == nil {
		return domain.ErrNotFound
	}
	return s.v2.UpsertPublishedModel(ctx, snapshot)
}

func (s *PublishedStore) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.GetPublishedModelByRef(ctx, ref)
}

func (s *PublishedStore) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (s *PublishedStore) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	if s == nil || s.v2 == nil {
		return nil, domain.ErrNotFound
	}
	return s.v2.FindLatestPublishedModelByModelCode(ctx, kind, code)
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
