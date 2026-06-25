package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
)

// RuleSetRef 解释模型引用，供绑定解析与执行路由使用。
type RuleSetRef struct {
	Kind    domain.RuleSetKind
	Code    string
	Version string
	Title   string
}

func (r RuleSetRef) IsEmpty() bool {
	return r.Kind == "" && r.Code == ""
}

// PublishedRuleSetReader 读取已发布规则集快照。
type PublishedRuleSetReader interface {
	GetPublishedByRef(ctx context.Context, ref RuleSetRef) (*domain.RuleSetSnapshot, error)
	FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.RuleSetSnapshot, error)
}

// PublishedRuleSetWriter 写入已发布规则集（seed / 管理链路使用）。
type PublishedRuleSetWriter interface {
	UpsertPublished(ctx context.Context, snapshot *domain.RuleSetSnapshot) error
}

// QuestionnaireRuleSetResolver 根据问卷版本解析解释模型引用。
type QuestionnaireRuleSetResolver interface {
	ResolveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (RuleSetRef, bool, error)
}

// RuleSetCatalog 规则资产读侧统一端口（静态 seed / Mongo 实现均可）。
type RuleSetCatalog interface {
	PublishedRuleSetReader
	QuestionnaireRuleSetResolver
}
