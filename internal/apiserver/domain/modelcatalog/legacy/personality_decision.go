package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// FallbackPersonalityDecisionKind 映射旧版 personality 算法身份到判定类型。
// 仅用于 migration 读路径（legacy ruleset / v1 envelope）；published 快照必须携带显式 decision.kind。
func FallbackPersonalityDecisionKind(algorithm binding.Algorithm) binding.DecisionKind {
	switch algorithm {
	case binding.AlgorithmMBTI:
		return binding.DecisionKindPoleComposition
	case binding.AlgorithmSBTI:
		return binding.DecisionKindNearestPattern
	case binding.AlgorithmBigFive:
		return binding.DecisionKindTraitProfile
	default:
		return binding.DecisionKindScoreRange
	}
}
