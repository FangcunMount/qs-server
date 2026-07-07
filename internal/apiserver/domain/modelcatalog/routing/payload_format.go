package routing

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

const (
	// v2 production payload formats.
	PayloadFormatAssessmentScaleV1         = "assessmentmodel.scale.v1"
	PayloadFormatPersonalityTypologyV1     = "assessmentmodel.personality.typology.v1"
	PayloadFormatBehavioralRatingDefaultV1 = "assessmentmodel.behavioral_rating.default.v1"
	PayloadFormatBehavioralRatingBrief2V1  = "assessmentmodel.behavioral_rating.brief2.v1"
	PayloadFormatCognitiveDefaultV1        = "assessmentmodel.cognitive.default.v1"
	PayloadFormatCognitiveSPMV1            = "assessmentmodel.cognitive.spm.v1"

	// Legacy read-only payload formats (migration / outbox drain).
	PayloadFormatScaleV1 = "ruleset.scale.v1"
	PayloadFormatMBTIV1  = "ruleset.mbti.v1"
	PayloadFormatSBTIV1  = "ruleset.sbti.v1"

	PayloadFormatScaleV1Legacy = "evaluationinput.scale.v1"
	PayloadFormatMBTIV1Legacy  = "evaluationinput.mbti.v1"
	PayloadFormatSBTIV1Legacy  = "evaluationinput.sbti.v1"
)

func IsScalePayloadFormat(format string) bool {
	switch format {
	case PayloadFormatAssessmentScaleV1,
		PayloadFormatScaleV1, PayloadFormatScaleV1Legacy:
		return true
	default:
		return false
	}
}

// IsMBTIPayloadFormat reports legacy MBTI payload formats only.
// v2 typology payloads must be distinguished by AlgorithmFromTypologyPayload.
func IsMBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatMBTIV1, PayloadFormatMBTIV1Legacy:
		return true
	default:
		return false
	}
}

// IsSBTIPayloadFormat reports legacy SBTI payload formats only.
// v2 typology payloads must be distinguished by AlgorithmFromTypologyPayload.
func IsSBTIPayloadFormat(format string) bool {
	switch format {
	case PayloadFormatSBTIV1, PayloadFormatSBTIV1Legacy:
		return true
	default:
		return false
	}
}

func IsPersonalityTypologyPayloadFormat(format string) bool {
	return format == PayloadFormatPersonalityTypologyV1
}

type typologyAlgorithmEnvelope struct {
	Algorithm identity.Algorithm `json:"algorithm"`
}

// AlgorithmFromTypologyPayload reads the algorithm identity from a v2 typology payload.
func AlgorithmFromTypologyPayload(payload []byte) (identity.Algorithm, error) {
	var envelope typologyAlgorithmEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("decode typology payload algorithm: %w", err)
	}
	if envelope.Algorithm == "" {
		return "", fmt.Errorf("typology payload algorithm is empty")
	}
	return envelope.Algorithm, nil
}

// PayloadFormatForBehavioralRating resolves the published payload format for a behavioral_rating algorithm.
func PayloadFormatForBehavioralRating(algorithm identity.Algorithm) string {
	switch algorithm {
	case identity.AlgorithmBrief2:
		return PayloadFormatBehavioralRatingBrief2V1
	case identity.AlgorithmBehavioralRatingDefault, "":
		return PayloadFormatBehavioralRatingDefaultV1
	default:
		return PayloadFormatBehavioralRatingDefaultV1
	}
}

// PayloadFormatForCognitive resolves the published payload format for a cognitive algorithm.
func PayloadFormatForCognitive(algorithm identity.Algorithm) string {
	switch algorithm {
	case identity.AlgorithmSPM:
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
func DraftPayloadFormatForModel(kind identity.Kind, algorithm identity.Algorithm) string {
	switch kind {
	case identity.KindBehavioralRating:
		return PayloadFormatForBehavioralRating(algorithm)
	case identity.KindCognitive:
		return PayloadFormatForCognitive(algorithm)
	default:
		return ""
	}
}
