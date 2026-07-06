package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// EvaluatorKey routes execution to a concrete evaluator implementation.
type EvaluatorKey struct {
	Kind      modelcatalog.Kind
	SubKind   modelcatalog.SubKind
	Algorithm modelcatalog.Algorithm
}

var (
	EvaluatorKeyScaleDefault = EvaluatorKey{
		Kind:      modelcatalog.KindScale,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: modelcatalog.AlgorithmScaleDefault,
	}
	EvaluatorKeyMBTI                = PersonalityTypologyKey(modelcatalog.AlgorithmMBTI)
	EvaluatorKeySBTI                = PersonalityTypologyKey(modelcatalog.AlgorithmSBTI)
	EvaluatorKeyBigFive             = PersonalityTypologyKey(modelcatalog.AlgorithmBigFive)
	EvaluatorKeyPersonalityTypology = EvaluatorKey{
		Kind:      modelcatalog.KindPersonality,
		SubKind:   modelcatalog.SubKindTypology,
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
	}
)

// PersonalityTypologyKey builds the execution routing key for a typology algorithm.
func PersonalityTypologyKey(algorithm modelcatalog.Algorithm) EvaluatorKey {
	return EvaluatorKey{
		Kind:      modelcatalog.KindPersonality,
		SubKind:   modelcatalog.SubKindTypology,
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
	if k.Kind != modelcatalog.KindPersonality || k.SubKind != modelcatalog.SubKindTypology {
		return false
	}
	switch k.Algorithm {
	case modelcatalog.AlgorithmMBTI, modelcatalog.AlgorithmSBTI, modelcatalog.AlgorithmBigFive:
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

func EvaluatorKeyFromLegacyKind(kind modelcatalog.Kind) (EvaluatorKey, bool) {
	mappedKind, subKind, algorithm, ok := modelcatalog.LegacyKindMapping(kind)
	if !ok {
		return EvaluatorKey{}, false
	}
	return EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}, true
}
