package identity

import "fmt"

// ProductChannel classifies an assessment model for product-facing taxonomy.
type ProductChannel string

const (
	ProductChannelMedicalScale    ProductChannel = "medical_scale"
	ProductChannelPersonality     ProductChannel = "personality"
	ProductChannelBehaviorAbility ProductChannel = "behavior_ability"
	ProductChannelCognitive       ProductChannel = "cognitive"
	ProductChannelCustom          ProductChannel = "custom"
)

func (pc ProductChannel) String() string { return string(pc) }

func (pc ProductChannel) IsValid() bool {
	switch pc {
	case ProductChannelMedicalScale,
		ProductChannelPersonality,
		ProductChannelBehaviorAbility,
		ProductChannelCognitive,
		ProductChannelCustom:
		return true
	default:
		return false
	}
}

// DefaultProductChannelFor derives the default product channel from a model family kind.
// This is a UI/create-form default only; it is not a domain constraint.
// Use ResolveProductChannel with an explicit channel when product taxonomy matters.
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

// ResolveProductChannel returns the explicit channel when set, otherwise the kind default.
func ResolveProductChannel(kind Kind, channel ProductChannel) ProductChannel {
	if channel != "" {
		return channel
	}
	return DefaultProductChannelFor(kind)
}

// CompleteProductChannel validates an optional product channel and applies kind defaults.
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

// AllProductChannels returns the supported product channel values for API options.
func AllProductChannels() []ProductChannel {
	return []ProductChannel{
		ProductChannelMedicalScale,
		ProductChannelPersonality,
		ProductChannelBehaviorAbility,
		ProductChannelCognitive,
		ProductChannelCustom,
	}
}
