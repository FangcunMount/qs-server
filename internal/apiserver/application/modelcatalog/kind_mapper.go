package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

const AlgorithmCustomTypology = "custom_typology"

func APIKindToDomainKind(kind string) (domain.Kind, bool) {
	switch kind {
	case KindTypology:
		return domain.KindTypology, true
	case string(domain.KindBehavioralRating):
		return domain.KindBehavioralRating, true
	case KindScale:
		return domain.KindScale, true
	case KindCognitive:
		return domain.KindCognitive, true
	default:
		return "", false
	}
}

// DomainKindToAPIKind 映射规范领域类型 到 外部 API 契约。
func DomainKindToAPIKind(kind domain.Kind) string {
	switch kind {
	case domain.KindTypology:
		return KindTypology
	case domain.KindBehavioralRating:
		return string(domain.KindBehavioralRating)
	case domain.KindScale:
		return KindScale
	case domain.KindCognitive:
		return KindCognitive
	default:
		return string(kind)
	}
}

// APIPayloadFormatToDomain 归一化API 载荷格式 到 规范领域格式。
func APIPayloadFormatToDomain(format string) string {
	return format
}

// DomainPayloadFormatToAPI 映射规范 领域载荷格式 back 到 API 值。
func DomainPayloadFormatToAPI(kind string, format string) string {
	return format
}

func IsSupportedAPIKind(kind string) bool {
	_, ok := APIKindToDomainKind(kind)
	return ok
}
