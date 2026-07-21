package assessment

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionIdentity 返回执行路由身份 用于 这个模型引用。
func (r EvaluationModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentity{Kind: modelcatalog.Kind(r.kind), SubKind: r.subKind, Algorithm: r.algorithm}
}
