package modelcatalog

// IsBehaviorAbilityProductChannelAPIKind reports whether apiKind is the behavior-ability product channel.
func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

// BehaviorAbilityChannelModelFamilies returns executable model families aggregated by the channel.
func BehaviorAbilityChannelModelFamilies() []Kind {
	return []Kind{KindBehavioralRating, KindCognitive}
}

// IsBehaviorAbilityChannelFamily reports whether kind is listed under the behavior-ability channel.
func IsBehaviorAbilityChannelFamily(kind Kind) bool {
	switch kind {
	case KindBehaviorAbility, KindBehavioralRating, KindCognitive:
		return true
	default:
		return false
	}
}

// ResolveBehaviorAbilityChannelFamily maps an optional channel filter to a model family kind.
func ResolveBehaviorAbilityChannelFamily(filter string) (Kind, bool) {
	switch Kind(filter) {
	case KindBehaviorAbility, KindBehavioralRating, KindCognitive:
		return Kind(filter), true
	case "":
		return "", false
	default:
		return "", false
	}
}
