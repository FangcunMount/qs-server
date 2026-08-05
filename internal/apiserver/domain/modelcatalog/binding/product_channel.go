package binding

// ProductChannel 划分assessment model 用于 product-facing 产品分类体系。
type ProductChannel string

const (
	ProductChannelMedicalScale    ProductChannel = "medical_scale"
	ProductChannelTypology        ProductChannel = "typology"
	ProductChannelBehaviorAbility ProductChannel = "behavior_ability"
)

// 默认ProductChannelFor 推导默认 产品通道 从 模型家族 类型。
// 这是UI/创建表单 默认 仅; 它是 不 领域 constraint。
// 使用 ResolveProductChannel 使用 显式 channel when product 分类体系 matters。
func DefaultProductChannelFor(kind Kind) ProductChannel {
	switch kind {
	case KindScale:
		return ProductChannelMedicalScale
	case KindTypology:
		return ProductChannelTypology
	case KindBehavioralRating:
		return ProductChannelBehaviorAbility
	case KindCognitive:
		return ProductChannelBehaviorAbility
	default:
		return ""
	}
}

// CanonicalSubKindFor derives the legacy sub-kind representation from the
// canonical Kind. It exists solely at compatibility boundaries; new catalog
// records must not persist it.
func CanonicalSubKindFor(kind Kind) SubKind {
	if kind == KindTypology {
		return SubKindTypology
	}
	return SubKindEmpty
}

// ResolveProductChannel 返回显式 channel when set, otherwise 类型 默认。
func ResolveProductChannel(kind Kind, channel ProductChannel) ProductChannel {
	if channel != "" {
		return channel
	}
	return DefaultProductChannelFor(kind)
}
