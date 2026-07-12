package assessment

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionIdentity 返回执行路由身份 用于 这个模型引用。
func (r EvaluationModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	if r.algorithm != "" {
		kind := modelcatalog.Kind(r.kind)
		if kind == modelcatalog.KindBehavioralRating {
			return evaldomain.ExecutionIdentityBehavioralRatingDefault
		}
		return evaldomain.ExecutionIdentity{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
	}
	if id, ok := evaldomain.ExecutionIdentityFromLegacyKind(modelcatalog.Kind(r.kind)); ok {
		return id
	}
	if modelcatalog.Kind(r.kind) == modelcatalog.KindBehavioralRating && r.algorithm == "" {
		return evaldomain.ExecutionIdentityBehavioralRatingDefault
	}
	if modelcatalog.Kind(r.kind) == modelcatalog.KindCognitive && r.algorithm == "" {
		return evaldomain.ExecutionIdentityCognitiveDefault
	}
	return evaldomain.ExecutionIdentity{
		Kind:      modelcatalog.Kind(r.kind),
		SubKind:   r.subKind,
		Algorithm: r.algorithm,
	}
}
