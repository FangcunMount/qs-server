package assessment

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionIdentity 返回执行路由身份 用于 这个模型引用。
func (r EvaluationModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	kind := modelcatalog.Kind(r.kind)
	if r.algorithm != "" {
		return evaldomain.ExecutionIdentity{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
	}
	if id, ok := evaldomain.ExecutionIdentityFromLegacyKind(kind); ok {
		return id
	}
	return evaldomain.ExecutionIdentity{Kind: kind, SubKind: r.subKind}
}
