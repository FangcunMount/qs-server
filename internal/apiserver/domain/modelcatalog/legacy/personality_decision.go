package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// FallbackPersonalityDecisionKind 映射旧版 personality 算法身份到判定类型。
// 仅用于 migration 读路径（legacy ruleset / v1 envelope）；published 快照必须携带显式 decision.kind。
func FallbackPersonalityDecisionKind(algorithm identity.Algorithm) identity.DecisionKind {
	switch algorithm {
	case identity.AlgorithmMBTI:
		return identity.DecisionKindPoleComposition
	case identity.AlgorithmSBTI:
		return identity.DecisionKindNearestPattern
	case identity.AlgorithmBigFive:
		return identity.DecisionKindTraitProfile
	default:
		return identity.DecisionKindScoreRange
	}
}
