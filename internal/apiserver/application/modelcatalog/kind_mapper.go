package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const AlgorithmCustomTypology = "custom_typology"

// APIKindToDomainKind 映射外部 API 类型值 到 规范领域类型。
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

// DomainKindToAPIKind 映射规范领域类型 到 外部 API 契约。
func DomainKindToAPIKind(kind domain.Kind) string {
	if entry, ok := catalogRegistry.ByKind(kind); ok && entry.APIKind != "" {
		return entry.APIKind
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

// APIPayloadFormatToDomain 归一化API 载荷格式 到 规范领域格式。
func APIPayloadFormatToDomain(format string) string {
	switch format {
	case PayloadFormatMedicalScaleV1:
		return domain.PayloadFormatAssessmentScaleV1
	default:
		return format
	}
}

// DomainPayloadFormatToAPI 映射规范 领域载荷格式 back 到 API 值。
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
	_, ok := catalogRegistry.ByAPIKind(kind)
	return ok
}
