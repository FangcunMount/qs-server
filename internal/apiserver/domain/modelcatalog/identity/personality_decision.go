package identity

// FallbackPersonalityDecisionKind 映射旧版 personality 算法 身份 到 判定类型s。
// Draft-仅 兼容性: 用于 草稿载荷 未声明 decision.类型 显式ly。
// Published 快照 必须 携带 显式 decision.类型; do 不 rely on 这个fallback at publish time。
func FallbackPersonalityDecisionKind(algorithm Algorithm) DecisionKind {
	switch algorithm {
	case AlgorithmMBTI:
		return DecisionKindPoleComposition
	case AlgorithmSBTI:
		return DecisionKindNearestPattern
	case AlgorithmBigFive:
		return DecisionKindTraitProfile
	default:
		return DecisionKindScoreRange
	}
}
