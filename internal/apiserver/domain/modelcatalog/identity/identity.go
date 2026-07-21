package identity

// Identity 是测评模型的算法身份，不表达产品概念。
type Identity struct {
	Kind      Kind
	SubKind   SubKind
	Algorithm Algorithm
}

func New(kind Kind, subKind SubKind, algorithm Algorithm) Identity {
	return Identity{Kind: kind, SubKind: subKind, Algorithm: algorithm}
}

func (i Identity) IsZero() bool {
	return i.Kind == "" && i.SubKind == "" && i.Algorithm == ""
}

func (i Identity) Family() (Family, bool) {
	return FamilyFromIdentity(i)
}

func (i Identity) DecisionKind() (DecisionKind, bool) {
	return DecisionKindForIdentity(i.Kind, i.SubKind, i.Algorithm)
}

// Family 是运行执行机制家族，始终由 Identity 或 DecisionKind 派生。
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

type Family = AlgorithmFamily

const (
	FamilyFactorScoring        Family = AlgorithmFamilyFactorScoring
	FamilyFactorClassification Family = AlgorithmFamilyFactorClassification
	FamilyFactorNorm           Family = AlgorithmFamilyFactorNorm
	FamilyTaskPerformance      Family = AlgorithmFamilyTaskPerformance
)

func FamilyFromDecisionKind(decision DecisionKind) (Family, bool) {
	return AlgorithmFamilyFromDecisionKind(decision)
}

func FamilyFromIdentity(identity Identity) (Family, bool) {
	return AlgorithmFamilyFromIdentity(identity.Kind, identity.SubKind, identity.Algorithm)
}

// AlgorithmFamilyFromDecisionKind 映射 published 判定策略到执行家族。
func AlgorithmFamilyFromDecisionKind(decision DecisionKind) (AlgorithmFamily, bool) {
	switch decision {
	case DecisionKindScoreRange:
		return AlgorithmFamilyFactorScoring, true
	case DecisionKindPoleComposition, DecisionKindTraitProfile, DecisionKindNearestPattern, DecisionKindDominantFactor:
		return AlgorithmFamilyFactorClassification, true
	case DecisionKindNormLookup:
		return AlgorithmFamilyFactorNorm, true
	case DecisionKindAbilityLevel:
		return AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// DecisionKindForIdentity mirrors publish-builder decision selection for non-typology draft binding.
// Personality typology requires explicit decision.kind in payload; no algorithm fallback.
// Cognitive/projection currently uses task_performance as the implementation family.
func DecisionKindForIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (DecisionKind, bool) {
	switch kind {
	case KindScale:
		return DecisionKindScoreRange, true
	case KindTypology:
		if subKind != SubKindTypology {
			return "", false
		}
		return "", false
	case KindBehavioralRating:
		return DecisionKindNormLookup, true
	case KindCognitive:
		return DecisionKindAbilityLevel, true
	default:
		return "", false
	}
}

// AlgorithmFamilyFromIdentity 推导执行家族 from draft model binding.
func AlgorithmFamilyFromIdentity(kind Kind, subKind SubKind, algorithm Algorithm) (AlgorithmFamily, bool) {
	if kind == KindTypology && subKind == SubKindTypology {
		return AlgorithmFamilyFactorClassification, true
	}
	decision, ok := DecisionKindForIdentity(kind, subKind, algorithm)
	if !ok {
		return "", false
	}
	return AlgorithmFamilyFromDecisionKind(decision)
}

// CompatibleAlgorithmBinding reports whether Kind/SubKind/Algorithm form a
// known ModelIdentity ↔ AlgorithmBinding matrix entry. Empty Algorithm is
// allowed for draft authoring; publish validators still enforce concrete values.
func CompatibleAlgorithmBinding(kind Kind, subKind SubKind, algorithm Algorithm) bool {
	switch kind {
	case KindScale:
		return algorithm == "" || algorithm == AlgorithmScaleDefault
	case KindTypology:
		if subKind != SubKindEmpty && subKind != SubKindTypology {
			return false
		}
		switch algorithm {
		case "", AlgorithmPersonalityTypology:
			return true
		default:
			return false
		}
	case KindBehavioralRating:
		switch algorithm {
		case "", AlgorithmBrief2, AlgorithmSPMSensory:
			return true
		default:
			return false
		}
	case KindCognitive:
		return algorithm == "" || algorithm == AlgorithmSPM
	default:
		return false
	}
}

// CompatibleIdentity is CompatibleAlgorithmBinding for an Identity value.
func CompatibleIdentity(identity Identity) bool {
	return CompatibleAlgorithmBinding(identity.Kind, identity.SubKind, identity.Algorithm)
}

// AllAlgorithmFamilies 返回 supported 算法家族 values 用于 API 选项。
func AllAlgorithmFamilies() []AlgorithmFamily {
	return []AlgorithmFamily{
		AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance,
	}
}

// AlgorithmFamilyStringFromIdentity derives the algorithm family string from model identity fields.
func AlgorithmFamilyStringFromIdentity(kind Kind, subKind SubKind, algorithm Algorithm) string {
	if kind == "" {
		return ""
	}
	family, ok := AlgorithmFamilyFromIdentity(kind, subKind, algorithm)
	if !ok {
		return ""
	}
	return string(family)
}
