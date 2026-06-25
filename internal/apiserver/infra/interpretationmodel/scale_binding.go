package interpretationmodel

import (
	"context"

	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ScaleBindingSource 从量表仓储解析已发布 scale 绑定（静态 catalog 回退）。
type ScaleBindingSource interface {
	FindScaleByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*evaluationinputPort.ScaleSnapshot, error)
	GetScaleByRef(ctx context.Context, code, version string) (*evaluationinputPort.ScaleSnapshot, error)
}
