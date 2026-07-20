package identity

// TypologyAlgorithmBackfillTarget returns the canonical algorithm for a retired
// typology alias (oneoff backfill only; runtime dual-identity is retired).
func TypologyAlgorithmBackfillTarget(algorithm Algorithm) (Algorithm, bool) {
	switch algorithm {
	case AlgorithmMBTI, AlgorithmSBTI, AlgorithmBigFive:
		return AlgorithmPersonalityTypology, true
	default:
		return "", false
	}
}

// CanonicalTypologyPublishAlgorithm is the only typology algorithm allowed on new publishes.
func CanonicalTypologyPublishAlgorithm() Algorithm {
	return AlgorithmPersonalityTypology
}

// BehavioralAlgorithmBackfillTarget picks the canonical publish algorithm for a
// retired behavioral_rating_default snapshot (oneoff backfill only).
func BehavioralAlgorithmBackfillTarget(algorithm Algorithm, hasBrief2Spec bool, hasNormRefs bool, preferredTarget Algorithm) (Algorithm, string, bool) {
	if algorithm != AlgorithmBehavioralRatingDefault {
		return "", "not_retained_read_alias", false
	}
	if hasBrief2Spec {
		if preferredTarget != "" && preferredTarget != AlgorithmBrief2 {
			return "", "brief2_spec_conflicts_with_target", false
		}
		return AlgorithmBrief2, "", true
	}
	switch preferredTarget {
	case AlgorithmBrief2, AlgorithmSPMSensory:
		if !hasNormRefs {
			return "", "requires_norm_refs_for_explicit_target", false
		}
		return preferredTarget, "", true
	case "":
		if hasNormRefs {
			return "", "ambiguous_brief2_or_spm_sensory", false
		}
		return "", "requires_brief2_execution_or_norm_refs", false
	default:
		return "", "unsupported_preferred_target", false
	}
}
