package modelcatalog

// AlgorithmFamily groups execution semantics for runtime, payload, and reporting.
// It is always derived from identity or DecisionKind and is never persisted.
type AlgorithmFamily string

const (
	AlgorithmFamilyFactorScoring        AlgorithmFamily = "factor_scoring"
	AlgorithmFamilyFactorClassification AlgorithmFamily = "factor_classification"
	AlgorithmFamilyFactorNorm           AlgorithmFamily = "factor_norm"
	AlgorithmFamilyTaskPerformance      AlgorithmFamily = "task_performance"
)

func (f AlgorithmFamily) String() string { return string(f) }

func (f AlgorithmFamily) IsValid() bool {
	switch f {
	case AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance:
		return true
	default:
		return false
	}
}

// AlgorithmFamilyFromDecisionKind maps a published decision strategy to its execution family.
func AlgorithmFamilyFromDecisionKind(decision DecisionKind) (AlgorithmFamily, bool) {
	switch decision {
	case DecisionKindScoreRange, DecisionKindScoreRangeInterpretation:
		return AlgorithmFamilyFactorScoring, true
	case DecisionKindPoleComposition, DecisionKindTraitProfile, DecisionKindNearestPattern:
		return AlgorithmFamilyFactorClassification, true
	case DecisionKindNormLookup:
		return AlgorithmFamilyFactorNorm, true
	case DecisionKindAbilityLevel:
		return AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// DecisionKindForIdentity mirrors publish-builder decision selection for draft identity.
func DecisionKindForIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (DecisionKind, bool) {
	switch kind {
	case KindScale:
		return DecisionKindScoreRange, true
	case KindPersonality:
		if subKind != SubKindTypology {
			return "", false
		}
		return personalityDecisionKindForAlgorithm(algorithm), true
	case KindBehavioralRating:
		algo := algorithm
		if algo == "" {
			algo = AlgorithmBrief2
		}
		if algo == AlgorithmBrief2 {
			return DecisionKindNormLookup, true
		}
		return DecisionKindScoreRange, true
	case KindCognitive:
		return DecisionKindScoreRange, true
	case KindBehaviorAbility, KindCustom:
		return "", false
	default:
		return "", false
	}
}

// AlgorithmFamilyFromIdentity derives the execution family from draft model identity.
func AlgorithmFamilyFromIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (AlgorithmFamily, bool) {
	if kind == KindBehaviorAbility {
		return "", false
	}
	decision, ok := DecisionKindForIdentity(kind, subKind, algorithm)
	if !ok {
		return "", false
	}
	return AlgorithmFamilyFromDecisionKind(decision)
}

// AllAlgorithmFamilies returns supported algorithm family values for API options.
func AllAlgorithmFamilies() []AlgorithmFamily {
	return []AlgorithmFamily{
		AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance,
	}
}
