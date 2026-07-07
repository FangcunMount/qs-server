package legacy

// KindBehaviorAbilityLegacy is the deprecated flat kind used as a product-channel API filter.
// New models must use behavioral_rating or cognitive instead.
const KindBehaviorAbilityLegacy = "behavior_ability"

// IsDeprecatedProductChannelKind reports legacy kinds that are product-channel slots only.
func IsDeprecatedProductChannelKind(kind string) bool {
	return kind == KindBehaviorAbilityLegacy
}
