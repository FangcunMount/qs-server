package ruleset

import (
	"context"

	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
)

// ScaleBindingSource 从量表仓储解析已发布 scale 绑定（静态 catalog 回退）。
type ScaleBindingSource interface {
	FindScaleByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*rulesetscale.ScaleSnapshot, error)
	GetScaleByRef(ctx context.Context, code, version string) (*rulesetscale.ScaleSnapshot, error)
}
