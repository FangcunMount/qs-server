package binding

// APIKindBehaviorAbility is the external API kind for the behavior-ability product channel.
// It aggregates behavioral_rating and cognitive model families in list/options APIs only.
const APIKindBehaviorAbility = "behavior_ability"

// IsBehaviorAbilityProductChannelAPIKind reports whether apiKind is the behavior-ability channel.
func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

// BehaviorAbilityChannelModelFamilies returns executable families aggregated by the channel.
func BehaviorAbilityChannelModelFamilies() []Kind {
	return []Kind{KindBehavioralRating, KindCognitive}
}

// IsBehaviorAbilityChannelFamily reports whether kind is listed under behavior-ability channel.
func IsBehaviorAbilityChannelFamily(kind Kind) bool {
	switch kind {
	case KindBehavioralRating, KindCognitive:
		return true
	default:
		return false
	}
}

// ResolveBehaviorAbilityChannelFamily maps optional channel filter to model family kind.
func ResolveBehaviorAbilityChannelFamily(filter string) (Kind, bool) {
	switch Kind(filter) {
	case KindBehavioralRating, KindCognitive:
		return Kind(filter), true
	case "":
		return "", false
	default:
		return "", false
	}
}
