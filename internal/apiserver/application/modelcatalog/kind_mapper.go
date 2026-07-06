package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const (
	AlgorithmScoreRange     = "score_range"
	AlgorithmCustomTypology = "custom_typology"
)

// APIKindToDomainKind maps external API kind values to canonical domain kinds.
func APIKindToDomainKind(kind string) (domain.Kind, bool) {
	switch kind {
	case KindPersonality:
		return domain.KindPersonality, true
	case KindBehaviorAbility:
		return domain.KindBehavioralRating, true
	case KindMedicalScale, "scale":
		return domain.KindScale, true
	case KindCognitive:
		return domain.KindCognitive, true
	case KindCustom:
		return domain.KindCustom, true
	default:
		return "", false
	}
}

// DomainKindToAPIKind maps canonical domain kinds to the external API contract.
func DomainKindToAPIKind(kind domain.Kind) string {
	switch kind {
	case domain.KindPersonality:
		return KindPersonality
	case domain.KindBehavioralRating:
		return KindBehaviorAbility
	case domain.KindScale:
		return KindMedicalScale
	case domain.KindCognitive:
		return KindCognitive
	case domain.KindCustom:
		return KindCustom
	default:
		return string(kind)
	}
}

func APIPayloadFormatToDomain(format string) string {
	switch format {
	case PayloadFormatScaleV1:
		return domain.PayloadFormatBehavioralRatingDefaultV1
	case PayloadFormatMedicalScaleV1:
		return domain.PayloadFormatAssessmentScaleV1
	default:
		return format
	}
}

func DomainPayloadFormatToAPI(kind string, format string) string {
	switch format {
	case domain.PayloadFormatBehavioralRatingDefaultV1:
		return PayloadFormatScaleV1
	case domain.PayloadFormatAssessmentScaleV1:
		if kind == KindBehaviorAbility {
			return PayloadFormatScaleV1
		}
		return PayloadFormatMedicalScaleV1
	default:
		return format
	}
}

func IsSupportedAPIKind(kind string) bool {
	_, ok := APIKindToDomainKind(kind)
	return ok
}
