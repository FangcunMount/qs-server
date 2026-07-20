package identity

// TypologyAlgorithmsEquivalent reports whether two typology algorithms share the
// configured runtime route (MC-R018 batch 3).
//
// Equivalence is only:
//   retained-read alias (mbti|sbti|bigfive) ↔ personality_typology
// Distinct retained aliases (e.g. mbti ↔ sbti) are NOT equivalent.
func TypologyAlgorithmsEquivalent(left, right Algorithm) bool {
	if left == right {
		return true
	}
	return typologyAliasPair(left, right) || typologyAliasPair(right, left)
}

func typologyAliasPair(alias, canonical Algorithm) bool {
	return IsRetainedReadAlgorithm(KindTypology, alias) && canonical == AlgorithmPersonalityTypology
}

// CanonicalTypologyPublishAlgorithm is the only typology algorithm allowed on new publishes.
func CanonicalTypologyPublishAlgorithm() Algorithm {
	return AlgorithmPersonalityTypology
}

// TypologyAlgorithmLookupAlternates returns additional algorithms to try when
// resolving a published typology snapshot by ref. Exact algorithm is tried first
// by the caller; these are fallbacks for dual-identity during backfill.
func TypologyAlgorithmLookupAlternates(algorithm Algorithm) []Algorithm {
	switch ClassifyAlgorithmWritePolicy(KindTypology, algorithm) {
	case AlgorithmWriteRetainedRead:
		return []Algorithm{AlgorithmPersonalityTypology}
	case AlgorithmWriteCanonical:
		if algorithm == AlgorithmPersonalityTypology {
			return []Algorithm{AlgorithmMBTI, AlgorithmSBTI, AlgorithmBigFive}
		}
	}
	return nil
}

// TypologyAlgorithmBackfillTarget returns the canonical algorithm for a retained-read alias.
func TypologyAlgorithmBackfillTarget(algorithm Algorithm) (Algorithm, bool) {
	if !IsRetainedReadAlgorithm(KindTypology, algorithm) {
		return "", false
	}
	return AlgorithmPersonalityTypology, true
}

// BehavioralAlgorithmsEquivalent reports dual-identity for behavioral_rating (MC-R018 batch 4).
//
// Equivalence is only:
//   behavioral_rating_default ↔ brief2
//   behavioral_rating_default ↔ spm_sensory
// brief2 and spm_sensory are NOT equivalent to each other.
func BehavioralAlgorithmsEquivalent(left, right Algorithm) bool {
	if left == right {
		return true
	}
	return behavioralAliasPair(left, right) || behavioralAliasPair(right, left)
}

func behavioralAliasPair(alias, canonical Algorithm) bool {
	if alias != AlgorithmBehavioralRatingDefault {
		return false
	}
	return canonical == AlgorithmBrief2 || canonical == AlgorithmSPMSensory
}

// BehavioralAlgorithmLookupAlternates returns alternate algorithms for published lookup.
func BehavioralAlgorithmLookupAlternates(algorithm Algorithm) []Algorithm {
	switch algorithm {
	case AlgorithmBehavioralRatingDefault:
		return []Algorithm{AlgorithmBrief2, AlgorithmSPMSensory}
	case AlgorithmBrief2, AlgorithmSPMSensory:
		return []Algorithm{AlgorithmBehavioralRatingDefault}
	default:
		return nil
	}
}

// BehavioralAlgorithmBackfillTarget picks the canonical publish algorithm for a
// retained behavioral_rating_default snapshot (MC-R018 batch 4).
//
// Auto-eligible:
//   - Execution.Brief2 present → brief2
// Ambiguous NormRefs-only snapshots require preferredTarget (brief2|spm_sensory).
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
