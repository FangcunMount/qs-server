package routing

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// AlgorithmFamily groups execution semantics for runtime, payload, and reporting.
// It is always derived from identity or DecisionKind and is never persisted.
//
// Package name vs enum (dual layer):
//
//	Go package          AlgorithmFamily
//	scoring             factor_scoring
//	typology            factor_classification
//	norming             factor_norm
//	task_performance    task_performance
//
// See docs/02-业务模块/mechanism-oriented-migration.md §包名与 AlgorithmFamily 对照表.
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
func AlgorithmFamilyFromDecisionKind(decision identity.DecisionKind) (AlgorithmFamily, bool) {
	switch decision {
	case identity.DecisionKindScoreRange, identity.DecisionKind("score_range_interpretation"):
		return AlgorithmFamilyFactorScoring, true
	case identity.DecisionKindPoleComposition, identity.DecisionKindTraitProfile, identity.DecisionKindNearestPattern:
		return AlgorithmFamilyFactorClassification, true
	case identity.DecisionKindNormLookup:
		return AlgorithmFamilyFactorNorm, true
	case identity.DecisionKindAbilityLevel:
		return AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// DecisionKindForIdentity mirrors publish-builder decision selection for draft identity.
func DecisionKindForIdentity(kind identity.Kind, subKind identity.SubKind, algorithm identity.Algorithm) (identity.DecisionKind, bool) {
	switch kind {
	case identity.KindScale:
		return identity.DecisionKindScoreRange, true
	case identity.KindPersonality:
		if subKind != identity.SubKindTypology {
			return "", false
		}
		return identity.FallbackPersonalityDecisionKind(algorithm), true
	case identity.KindBehavioralRating:
		algo := algorithm
		if algo == "" {
			algo = identity.AlgorithmBrief2
		}
		if algo == identity.AlgorithmBrief2 {
			return identity.DecisionKindNormLookup, true
		}
		return identity.DecisionKindScoreRange, true
	case identity.KindCognitive:
		return identity.DecisionKindScoreRange, true
	case identity.KindCustom:
		return "", false
	default:
		return "", false
	}
}

// AlgorithmFamilyFromIdentity derives the execution family from draft model identity.
func AlgorithmFamilyFromIdentity(kind identity.Kind, subKind identity.SubKind, algorithm identity.Algorithm) (AlgorithmFamily, bool) {
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
