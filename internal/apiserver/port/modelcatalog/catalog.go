package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Ref 测评模型引用，供绑定解析与执行路由使用。
type Ref struct {
	Kind      domain.Kind
	SubKind   domain.SubKind
	Algorithm domain.Algorithm
	Code      string
	Version   string
	Title     string
}

func (r Ref) IsEmpty() bool {
	return r.Kind == "" && r.Code == ""
}

// ListPublishedFilter narrows published-model list queries for C-side catalogs.
type ListPublishedFilter struct {
	Kind      domain.Kind
	SubKind   domain.SubKind
	Algorithm domain.Algorithm
	Category  string
	Page      int
	PageSize  int
}

// PublishedWriter writes v2 published assessment models (seed / admin paths).
type PublishedWriter interface {
	UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error
}

// PublishedModelReader reads v2 published assessment model snapshots for runtime execution.
// Callers must treat this as the only read path for C-side and evaluation flows.
// Draft models in ModelRepository must never be used for execution.
type PublishedModelReader interface {
	GetPublishedModelByRef(ctx context.Context, ref Ref) (*domain.PublishedModelSnapshot, error)
	FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.PublishedModelSnapshot, error)
}

// PublishedModelLister lists v2 published assessment model snapshots for C-side catalogs.
// FindPublishedModelByCode returns the latest published snapshot for a model code.
type PublishedModelLister interface {
	FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error)
	ListPublishedModels(ctx context.Context, filter ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error)
}

// PublishedAlgorithmLister lists distinct published personality typology algorithms.
type PublishedAlgorithmLister interface {
	ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error)
}

// QuestionnaireResolver 根据问卷版本解析测评模型引用。
type QuestionnaireResolver interface {
	ResolveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (Ref, bool, error)
}

// Catalog is the runtime read port for published assessment models.
type Catalog interface {
	PublishedModelReader
	QuestionnaireResolver
}
