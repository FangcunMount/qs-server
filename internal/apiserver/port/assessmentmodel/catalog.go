package assessmentmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
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

// RuleSetRef is kept as a compatibility name while callers migrate to Ref.
type RuleSetRef = Ref

// ListPublishedFilter narrows published-model list queries for C-side catalogs.
type ListPublishedFilter struct {
	Kind      domain.Kind
	SubKind   domain.SubKind
	Algorithm domain.Algorithm
	Category  string
	Page      int
	PageSize  int
}

// PublishedReader 读取已发布测评模型快照。
type PublishedReader interface {
	GetPublishedByRef(ctx context.Context, ref Ref) (*domain.Snapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error)
}

// PublishedLister lists published models for C-side catalogs.
type PublishedLister interface {
	FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error)
	ListPublished(ctx context.Context, filter ListPublishedFilter) ([]*domain.Snapshot, int64, error)
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

// PublishedRuleSetReader is kept as a compatibility name while callers migrate to PublishedReader.
type PublishedRuleSetReader = PublishedReader

// PublishedWriter 写入已发布测评模型（seed / 管理链路使用）。
type PublishedWriter interface {
	UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error
}

// PublishedRuleSetWriter is kept as a compatibility name while callers migrate to PublishedWriter.
type PublishedRuleSetWriter = PublishedWriter

// QuestionnaireResolver 根据问卷版本解析测评模型引用。
type QuestionnaireResolver interface {
	ResolveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (Ref, bool, error)
}

// QuestionnaireRuleSetResolver is kept as a compatibility name while callers migrate to QuestionnaireResolver.
type QuestionnaireRuleSetResolver = QuestionnaireResolver

// Catalog 测评模型资产读侧统一端口（静态 seed / Mongo 实现均可）。
type Catalog interface {
	PublishedReader
	QuestionnaireResolver
}

// RuleSetCatalog is kept as a compatibility name while callers migrate to Catalog.
type RuleSetCatalog = Catalog
