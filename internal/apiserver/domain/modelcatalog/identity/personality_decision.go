package identity

// FallbackPersonalityDecisionKind maps legacy personality algorithm identities to decision kinds.
// Draft-only compatibility: used when draft payloads do not declare decision.kind explicitly.
// Published snapshots must carry an explicit decision.kind; do not rely on this fallback at publish time.
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
