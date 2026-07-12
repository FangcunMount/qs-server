package binding

import "fmt"

// ProductChannel 划分assessment model 用于 product-facing 产品分类体系。
type ProductChannel string

const (
	ProductChannelMedicalScale    ProductChannel = "medical_scale"
	ProductChannelTypology        ProductChannel = "typology"
	ProductChannelBehaviorAbility ProductChannel = "behavior_ability"
	ProductChannelScreening       ProductChannel = "screening"
	ProductChannelFollowup        ProductChannel = "followup"
)

func (pc ProductChannel) String() string { return string(pc) }

func (pc ProductChannel) IsValid() bool {
	switch pc {
	case ProductChannelMedicalScale,
		ProductChannelTypology,
		ProductChannelBehaviorAbility:
		return true
	default:
		return false
	}
}

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

// ResolveProductChannel 返回显式 channel when set, otherwise 类型 默认。
func ResolveProductChannel(kind Kind, channel ProductChannel) ProductChannel {
	if channel != "" {
		return channel
	}
	return DefaultProductChannelFor(kind)
}

// CompleteProductChannel 校验可选 产品通道 和 applies 类型 默认s。
func CompleteProductChannel(kind Kind, channel ProductChannel) (ProductChannel, error) {
	resolved := ResolveProductChannel(kind, channel)
	if resolved == "" {
		return "", fmt.Errorf("%w: product channel cannot be resolved for kind %s", ErrInvalidArgument, kind)
	}
	if !resolved.IsValid() {
		return "", fmt.Errorf("%w: product channel %q is invalid", ErrInvalidArgument, channel)
	}
	return resolved, nil
}

// ValidateNewProductChannel applies the current product taxonomy only to new
// drafts. CompleteProductChannel intentionally remains compatible with old
// rows whose historical channel was broader than their runtime kind.
func ValidateNewProductChannel(kind Kind, channel ProductChannel) error {
	resolved, err := CompleteProductChannel(kind, channel)
	if err != nil {
		return err
	}
	if (kind == KindBehavioralRating || kind == KindCognitive) && resolved != ProductChannelBehaviorAbility {
		return fmt.Errorf("%w: product channel %q is incompatible with kind %s", ErrInvalidArgument, resolved, kind)
	}
	return nil
}

// AllProductChannels 返回可配置 产品通道 values 用于 API 选项。
func AllProductChannels() []ProductChannel {
	return []ProductChannel{
		ProductChannelMedicalScale,
		ProductChannelTypology,
		ProductChannelBehaviorAbility,
	}
}
