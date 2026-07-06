package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// EvaluatorKey returns the v2 execution routing key for this model reference.
func (r EvaluationModelRef) EvaluatorKey() evaldomain.EvaluatorKey {
	if r.algorithm != "" {
		kind := modelcatalog.Kind(r.kind)
		if r.subKind != "" {
			return evaldomain.EvaluatorKey{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
		}
		if mappedKind, subKind, _, ok := modelcatalog.LegacyKindMapping(kind); ok {
			return evaldomain.EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: r.algorithm}
		}
		return evaldomain.EvaluatorKey{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
	}
	if key, ok := evaldomain.EvaluatorKeyFromLegacyKind(modelcatalog.Kind(r.kind)); ok {
		return key
	}
	return evaldomain.EvaluatorKey{
		Kind:      modelcatalog.Kind(r.kind),
		SubKind:   r.subKind,
		Algorithm: r.algorithm,
	}
}
