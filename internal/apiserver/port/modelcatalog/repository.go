package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ListFilter narrows draft-model list queries for admin consoles.
type ListFilter struct {
	Kind      domain.Kind
	SubKind   domain.SubKind
	Status    domain.ModelStatus
	Keyword   string
	Category  string
	Algorithm domain.Algorithm
	Page      int
	PageSize  int
}

// ModelRepository persists draft assessment models.
type ModelRepository interface {
	Create(ctx context.Context, model *domain.AssessmentModel) error
	Update(ctx context.Context, model *domain.AssessmentModel) error
	FindByCode(ctx context.Context, code string) (*domain.AssessmentModel, error)
	List(ctx context.Context, filter ListFilter) ([]*domain.AssessmentModel, int64, error)
	Delete(ctx context.Context, code string) error
}

// PublishedModelRepository persists published model runtime records for admin publish flows.
type PublishedModelRepository interface {
	Save(ctx context.Context, model *PublishedModel) error
	FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*PublishedModel, error)
	FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*PublishedModel, error)
	FindPublishedByModelCodeVersion(ctx context.Context, kind domain.Kind, code, version string) (*PublishedModel, error)
	ListPublished(ctx context.Context, filter ListPublishedFilter) ([]*PublishedModel, int64, error)
	DeletePublished(ctx context.Context, kind domain.Kind, code string) error
}
