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
	EvaluatorKeyMBTI                = PersonalityTypologyKey(assessmentmodel.AlgorithmMBTI)
	EvaluatorKeySBTI                = PersonalityTypologyKey(assessmentmodel.AlgorithmSBTI)
	EvaluatorKeyBigFive             = PersonalityTypologyKey(assessmentmodel.AlgorithmBigFive)
	EvaluatorKeyPersonalityTypology = EvaluatorKey{
		Kind:      assessmentmodel.KindPersonality,
		SubKind:   assessmentmodel.SubKindTypology,
		Algorithm: assessmentmodel.AlgorithmPersonalityTypology,
	}
)

// PersonalityTypologyKey builds the execution routing key for a typology algorithm.
func PersonalityTypologyKey(algorithm assessmentmodel.Algorithm) EvaluatorKey {
	return EvaluatorKey{
		Kind:      assessmentmodel.KindPersonality,
		SubKind:   assessmentmodel.SubKindTypology,
		Algorithm: algorithm,
	}
}

func (k EvaluatorKey) String() string {
	if k.SubKind == "" && k.Algorithm == "" {
		return k.Kind.String()
	}
	return k.Kind.String() + "/" + k.SubKind.String() + "/" + k.Algorithm.String()
}

func (k EvaluatorKey) IsZero() bool {
	return k.Kind == "" && k.SubKind == "" && k.Algorithm == ""
}

// IsPersonalityTypologyLegacyKey reports whether key is a built-in typology algorithm alias.
func (k EvaluatorKey) IsPersonalityTypologyLegacyKey() bool {
	if k.Kind != assessmentmodel.KindPersonality || k.SubKind != assessmentmodel.SubKindTypology {
		return false
	}
	switch k.Algorithm {
	case assessmentmodel.AlgorithmMBTI, assessmentmodel.AlgorithmSBTI, assessmentmodel.AlgorithmBigFive:
		return true
	default:
		return false
	}
}

// PersonalityTypologyLegacyKeys returns built-in typology algorithm routing keys.
func PersonalityTypologyLegacyKeys() []EvaluatorKey {
	return []EvaluatorKey{
		EvaluatorKeyMBTI,
		EvaluatorKeySBTI,
		EvaluatorKeyBigFive,
	}
}

// ResolvePersonalityTypologyExecutorKey maps legacy typology keys to the configured runtime key.
func ResolvePersonalityTypologyExecutorKey(key EvaluatorKey) EvaluatorKey {
	if key == EvaluatorKeyPersonalityTypology || key.IsPersonalityTypologyLegacyKey() {
		return EvaluatorKeyPersonalityTypology
	}
	return key
}

func EvaluatorKeyFromLegacyKind(kind assessmentmodel.Kind) (EvaluatorKey, bool) {
	mappedKind, subKind, algorithm, ok := assessmentmodel.LegacyKindMapping(kind)
	if !ok {
		return EvaluatorKey{}, false
	}
	return EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}, true
}
