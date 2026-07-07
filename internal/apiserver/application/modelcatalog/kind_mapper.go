package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const AlgorithmCustomTypology = "custom_typology"

// APIKindToDomainKind maps external API kind values to canonical domain kinds.
func APIKindToDomainKind(kind string) (domain.Kind, bool) {
	switch kind {
	case KindPersonality:
		return domain.KindPersonality, true
	case string(domain.KindBehavioralRating):
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
	cap, ok := domain.CapabilityByKind(kind)
	if ok && cap.APIKind != "" {
		return cap.APIKind
	}
	switch kind {
	case domain.KindPersonality:
		return KindPersonality
	case domain.KindBehavioralRating:
		return string(domain.KindBehavioralRating)
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

// APIPayloadFormatToDomain normalizes API payload formats to canonical domain formats.
func APIPayloadFormatToDomain(format string) string {
	switch format {
	case PayloadFormatMedicalScaleV1:
		return domain.PayloadFormatAssessmentScaleV1
	default:
		return format
	}
}

// DomainPayloadFormatToAPI maps canonical domain payload formats back to API values.
func DomainPayloadFormatToAPI(kind string, format string) string {
	switch format {
	case domain.PayloadFormatAssessmentScaleV1:
		return PayloadFormatMedicalScaleV1
	default:
		return format
	}
}

func IsSupportedAPIKind(kind string) bool {
	if domain.IsBehaviorAbilityProductChannelAPIKind(kind) {
		return true
	}
	_, ok := capabilityForAPIKind(kind)
	return ok
}
