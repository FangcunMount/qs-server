package publishing

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// AlgorithmFamily 分组执行语义 用于 运行时, 载荷, 和 reporting。
// 它是always 派生 从 身份 或 判定类型 和 是 never persisted。
//
// 包名与枚举值（双层）：
//
// Go 包 算法家族。
// 计分 因子_计分。
// 类型学 因子_分类。
// 常模ing 因子_常模。
// task_performance task_performance。
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

// AlgorithmFamilyFromDecisionKind 映射published 判定策略 到 its 执行家族。
func AlgorithmFamilyFromDecisionKind(decision binding.DecisionKind) (AlgorithmFamily, bool) {
	switch decision {
	case binding.DecisionKindScoreRange, binding.DecisionKind("score_range_interpretation"):
		return AlgorithmFamilyFactorScoring, true
	case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern:
		return AlgorithmFamilyFactorClassification, true
	case binding.DecisionKindNormLookup:
		return AlgorithmFamilyFactorNorm, true
	case binding.DecisionKindAbilityLevel:
		return AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// DecisionKindForIdentity mirrors publish-builder decision selection for non-typology draft binding.
// Personality typology requires explicit decision.kind in payload; no algorithm fallback.
func DecisionKindForIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) (binding.DecisionKind, bool) {
	switch kind {
	case binding.KindScale:
		return binding.DecisionKindScoreRange, true
	case binding.KindPersonality:
		if subKind != binding.SubKindTypology {
			return "", false
		}
		return "", false
	case binding.KindBehavioralRating:
		algo := algorithm
		if algo == "" {
			algo = binding.AlgorithmBrief2
		}
		if algo == binding.AlgorithmBrief2 {
			return binding.DecisionKindNormLookup, true
		}
		return binding.DecisionKindScoreRange, true
	case binding.KindCognitive:
		return binding.DecisionKindScoreRange, true
	case binding.KindCustom:
		return "", false
	default:
		return "", false
	}
}

// AlgorithmFamilyFromIdentity 推导执行家族 from draft model binding.
func AlgorithmFamilyFromIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) (AlgorithmFamily, bool) {
	if kind == binding.KindPersonality && subKind == binding.SubKindTypology {
		return AlgorithmFamilyFactorClassification, true
	}
	decision, ok := DecisionKindForIdentity(kind, subKind, algorithm)
	if !ok {
		return "", false
	}
	return AlgorithmFamilyFromDecisionKind(decision)
}

// AllAlgorithmFamilies 返回supported 算法家族 values 用于 API 选项。
func AllAlgorithmFamilies() []AlgorithmFamily {
	return []AlgorithmFamily{
		AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance,
	}
}
