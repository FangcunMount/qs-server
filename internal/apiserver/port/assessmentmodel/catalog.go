package assessmentmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

// Ref 测评模型引用，供绑定解析与执行路由使用。
type Ref struct {
	Kind    domain.Kind
	Code    string
	Version string
	Title   string
}

func (r Ref) IsEmpty() bool {
	return r.Kind == "" && r.Code == ""
}

// RuleSetRef is kept as a compatibility name while callers migrate to Ref.
type RuleSetRef = Ref

// PublishedReader 读取已发布测评模型快照。
type PublishedReader interface {
	GetPublishedByRef(ctx context.Context, ref Ref) (*domain.Snapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error)
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
