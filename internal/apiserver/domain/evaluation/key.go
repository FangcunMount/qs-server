package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"

// EvaluatorKey routes execution to a concrete evaluator implementation.
type EvaluatorKey struct {
	Kind      assessmentmodel.Kind
	SubKind   assessmentmodel.SubKind
	Algorithm assessmentmodel.Algorithm
}

var (
	EvaluatorKeyScaleDefault = EvaluatorKey{
		Kind:      assessmentmodel.KindScale,
		SubKind:   assessmentmodel.SubKindEmpty,
		Algorithm: assessmentmodel.AlgorithmScaleDefault,
	}
	EvaluatorKeyMBTI = EvaluatorKey{
		Kind:      assessmentmodel.KindPersonality,
		SubKind:   assessmentmodel.SubKindTypology,
		Algorithm: assessmentmodel.AlgorithmMBTI,
	}
	EvaluatorKeySBTI = EvaluatorKey{
		Kind:      assessmentmodel.KindPersonality,
		SubKind:   assessmentmodel.SubKindTypology,
		Algorithm: assessmentmodel.AlgorithmSBTI,
	}
)

func (k EvaluatorKey) String() string {
	if k.SubKind == "" && k.Algorithm == "" {
		return k.Kind.String()
	}
	return k.Kind.String() + "/" + k.SubKind.String() + "/" + k.Algorithm.String()
}

func (k EvaluatorKey) IsZero() bool {
	return k.Kind == "" && k.SubKind == "" && k.Algorithm == ""
}

func EvaluatorKeyFromLegacyKind(kind assessmentmodel.Kind) (EvaluatorKey, bool) {
	mappedKind, subKind, algorithm, ok := assessmentmodel.LegacyKindMapping(kind)
	if !ok {
		return EvaluatorKey{}, false
	}
	return EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}, true
}
