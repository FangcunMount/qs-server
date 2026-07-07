package assessment

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// EvaluatorKey returns the v2 execution routing key for this model reference.
func (r EvaluationModelRef) EvaluatorKey() evaldomain.EvaluatorKey {
	if r.algorithm != "" {
		kind := modelcatalog.Kind(r.kind)
		if r.subKind != "" {
			key := evaldomain.EvaluatorKey{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
			return evaldomain.ResolveBehavioralRatingExecutorKey(key)
		}
		if mappedKind, subKind, _, ok := modelcatalog.LegacyKindMapping(kind); ok {
			key := evaldomain.EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: r.algorithm}
			return evaldomain.ResolveBehavioralRatingExecutorKey(key)
		}
		key := evaldomain.EvaluatorKey{Kind: kind, SubKind: r.subKind, Algorithm: r.algorithm}
		return evaldomain.ResolveBehavioralRatingExecutorKey(key)
	}
	if key, ok := evaldomain.EvaluatorKeyFromLegacyKind(modelcatalog.Kind(r.kind)); ok {
		return key
	}
	if modelcatalog.Kind(r.kind) == modelcatalog.KindBehavioralRating && r.algorithm == "" {
		return evaldomain.EvaluatorKeyBehavioralRatingDefault
	}
	if modelcatalog.Kind(r.kind) == modelcatalog.KindCognitive && r.algorithm == "" {
		return evaldomain.EvaluatorKeyCognitiveDefault
	}
	return evaldomain.EvaluatorKey{
		Kind:      modelcatalog.Kind(r.kind),
		SubKind:   r.subKind,
		Algorithm: r.algorithm,
	}
}
