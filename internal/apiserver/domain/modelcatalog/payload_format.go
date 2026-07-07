package modelcatalog

// PayloadFormatForBehavioralRating resolves the published payload format for a behavioral_rating algorithm.
func PayloadFormatForBehavioralRating(algorithm Algorithm) string {
	switch algorithm {
	case AlgorithmBrief2:
		return PayloadFormatBehavioralRatingBrief2V1
	case AlgorithmBehavioralRatingDefault, "":
		return PayloadFormatBehavioralRatingDefaultV1
	default:
		return PayloadFormatBehavioralRatingDefaultV1
	}
}

// PayloadFormatForCognitive resolves the published payload format for a cognitive algorithm.
func PayloadFormatForCognitive(algorithm Algorithm) string {
	switch algorithm {
	case AlgorithmSPM:
		return PayloadFormatCognitiveSPMV1
	default:
		return PayloadFormatCognitiveDefaultV1
	}
}

// IsBehavioralRatingPayloadFormat reports whether format is a supported behavioral_rating payload.
func IsBehavioralRatingPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatBehavioralRatingDefaultV1, PayloadFormatBehavioralRatingBrief2V1:
		return true
	default:
		return false
	}
}

// IsCognitivePayloadFormat reports whether format is a supported cognitive payload.
func IsCognitivePayloadFormat(format string) bool {
	switch format {
	case PayloadFormatCognitiveDefaultV1, PayloadFormatCognitiveSPMV1:
		return true
	default:
		return false
	}
}

// DraftPayloadFormatForModel returns the draft/publish payload format for a model family and algorithm.
func DraftPayloadFormatForModel(kind Kind, algorithm Algorithm) string {
	switch kind {
	case KindBehavioralRating:
		return PayloadFormatForBehavioralRating(algorithm)
	case KindCognitive:
		return PayloadFormatForCognitive(algorithm)
	default:
		return ""
	}
}
