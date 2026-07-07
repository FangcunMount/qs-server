package modelcatalog

// FallbackPersonalityDecisionKind maps legacy personality algorithm identities to decision kinds.
// Compatibility-only: used when draft/default payloads do not declare decision.kind explicitly.
// Target state: read DecisionKind from model definition or published snapshot payload.
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
