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
