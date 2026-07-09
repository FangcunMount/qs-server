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

// AssessmentSnapshot 是已发布测评模型的不可变运行快照。
//
// 它是查询、缓存、评估执行共享的 read model/value object，不是 domain aggregate。
// 字段形态直接对应 published_assessment_models 的现有 BSON 契约。
type AssessmentSnapshot struct {
	SchemaVersion        string
	PayloadFormat        string
	ProductChannel       domain.ProductChannel
	Kind                 domain.Kind
	SubKind              domain.SubKind
	Algorithm            domain.Algorithm
	Code                 string
	Version              string
	Title                string
	Status               string
	DecisionKind         domain.DecisionKind
	QuestionnaireCode    string
	QuestionnaireVersion string
	Source               map[string]any
	Payload              []byte
}

// PublishedModel 是 AssessmentSnapshot 的兼容名称。
//
// Deprecated: use AssessmentSnapshot in new application/runtime code. The old
// name is retained because REST/gRPC behavior, Mongo fields, and existing
// repository interfaces still use "published model" terminology.
type PublishedModel = AssessmentSnapshot

// PublishedWriter writes v2 published assessment models (seed / admin paths).
type PublishedWriter interface {
	UpsertPublishedModel(ctx context.Context, model *PublishedModel) error
}

// PublishedModelReader reads v2 published assessment model records for runtime execution.
// Callers must treat this as the only read path for C-side and evaluation flows.
// Draft models in ModelRepository must never be used for execution.
type PublishedModelReader interface {
	GetPublishedModelByRef(ctx context.Context, ref Ref) (*PublishedModel, error)
	FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*PublishedModel, error)
}

// PublishedModelLister lists v2 published assessment model records for C-side catalogs.
// FindPublishedModelByCode returns the latest published record for a model code.
type PublishedModelLister interface {
	FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*PublishedModel, error)
	ListPublishedModels(ctx context.Context, filter ListPublishedFilter) ([]*PublishedModel, int64, error)
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
