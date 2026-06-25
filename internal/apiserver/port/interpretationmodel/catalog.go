package interpretationmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
)

// ModelRef 解释模型引用，供绑定解析与执行路由使用。
type ModelRef struct {
	Kind    domain.ModelKind
	Code    string
	Version string
	Title   string
}

func (r ModelRef) IsEmpty() bool {
	return r.Kind == "" && r.Code == ""
}

// PublishedModelReader 读取已发布规则集快照。
type PublishedModelReader interface {
	GetPublishedByRef(ctx context.Context, ref ModelRef) (*domain.RuleSetSnapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.RuleSetSnapshot, error)
}

// PublishedModelWriter 写入已发布规则集（seed / 管理链路使用）。
type PublishedModelWriter interface {
	UpsertPublished(ctx context.Context, snapshot *domain.RuleSetSnapshot) error
}

// QuestionnaireModelBindingResolver 根据问卷版本解析解释模型引用。
type QuestionnaireModelBindingResolver interface {
	ResolveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (ModelRef, bool, error)
}

// ModelCatalog 规则资产读侧统一端口（静态 seed / Mongo 实现均可）。
type ModelCatalog interface {
	PublishedModelReader
	QuestionnaireModelBindingResolver
}
