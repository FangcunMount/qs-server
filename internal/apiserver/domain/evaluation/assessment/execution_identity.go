package assessment

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionIdentity 返回执行路由身份 用于 这个模型引用。
func (r EvaluationModelRef) ExecutionIdentity() evaldomain.ExecutionIdentity {
	if r.algorithm != "" {
		kind := modelcatalog.Kind(r.kind)
		if r.subKind != "" {
			id := evaldomain.ExecutionIdentity{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
			return evaldomain.ResolveBehavioralRatingExecutorIdentity(id)
		}
		if mappedKind, subKind, _, ok := modelcatalog.LegacyKindMapping(kind); ok {
			id := evaldomain.ExecutionIdentity{Kind: mappedKind, SubKind: subKind, Algorithm: r.algorithm}
			return evaldomain.ResolveBehavioralRatingExecutorIdentity(id)
		}
		id := evaldomain.ExecutionIdentity{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
		return evaldomain.ResolveBehavioralRatingExecutorIdentity(id)
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

// EvaluatorKey 是deprecated; 使用 Execution身份。
func (r EvaluationModelRef) EvaluatorKey() evaldomain.ExecutionIdentity {
	return r.ExecutionIdentity()
}
