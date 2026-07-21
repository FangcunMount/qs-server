package identity

// CanonicalTypologyPublishAlgorithm is the only typology algorithm allowed on new publishes.
func CanonicalTypologyPublishAlgorithm() Algorithm {
	return AlgorithmPersonalityTypology
}
