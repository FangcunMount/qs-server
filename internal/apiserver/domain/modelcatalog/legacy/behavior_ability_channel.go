package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// IsBehaviorAbilityProductChannelAPIKind reports whether apiKind is the behavior-ability product channel.
func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

// BehaviorAbilityChannelModelFamilies returns executable model families aggregated by the channel.
func BehaviorAbilityChannelModelFamilies() []identity.Kind {
	return []identity.Kind{identity.KindBehavioralRating, identity.KindCognitive}
}

// IsBehaviorAbilityChannelFamily reports whether kind is listed under the behavior-ability channel.
func IsBehaviorAbilityChannelFamily(kind identity.Kind) bool {
	switch kind {
	case identity.KindBehavioralRating, identity.KindCognitive:
		return true
	default:
		return false
	}
}

// ResolveBehaviorAbilityChannelFamily maps an optional channel filter to a model family kind.
func ResolveBehaviorAbilityChannelFamily(filter string) (identity.Kind, bool) {
	switch identity.Kind(filter) {
	case identity.KindBehavioralRating, identity.KindCognitive:
		return identity.Kind(filter), true
	case "":
		return "", false
	default:
		return "", false
	}
}
