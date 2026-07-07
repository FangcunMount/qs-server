package routing

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
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

// DecisionKindForIdentity mirrors publish-builder decision 选择 用于 draft 身份。
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

// AlgorithmFamilyFromIdentity 推导执行家族 从 draft 模型身份。
func AlgorithmFamilyFromIdentity(kind identity.Kind, subKind identity.SubKind, algorithm identity.Algorithm) (AlgorithmFamily, bool) {
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
