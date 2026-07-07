package identity

import "fmt"

// ProductChannel 划分assessment model 用于 product-facing 分类体系。
type ProductChannel string

const (
	ProductChannelMedicalScale    ProductChannel = "medical_scale"
	ProductChannelPersonality     ProductChannel = "personality"
	ProductChannelBehaviorAbility ProductChannel = "behavior_ability"
	ProductChannelCognitive       ProductChannel = "cognitive"
	ProductChannelScreening       ProductChannel = "screening"
	ProductChannelFollowup        ProductChannel = "followup"
	ProductChannelCustom          ProductChannel = "custom"
)

func (pc ProductChannel) String() string { return string(pc) }

func (pc ProductChannel) IsValid() bool {
	switch pc {
	case ProductChannelMedicalScale,
		ProductChannelPersonality,
		ProductChannelBehaviorAbility,
		ProductChannelCognitive,
		ProductChannelScreening,
		ProductChannelFollowup,
		ProductChannelCustom:
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
	case KindPersonality:
		return ProductChannelPersonality
	case KindBehavioralRating:
		return ProductChannelBehaviorAbility
	case KindCognitive:
		return ProductChannelCognitive
	case KindCustom:
		return ProductChannelCustom
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

// AllProductChannels 返回supported 产品通道 values 用于 API 选项。
func AllProductChannels() []ProductChannel {
	return []ProductChannel{
		ProductChannelMedicalScale,
		ProductChannelPersonality,
		ProductChannelBehaviorAbility,
		ProductChannelCognitive,
		ProductChannelScreening,
		ProductChannelFollowup,
		ProductChannelCustom,
	}
}
