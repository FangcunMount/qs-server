package option

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// APIKindBehaviorAbility is the product API channel that presents behavioral-rating
// and cognitive model families together in catalog options.
const APIKindBehaviorAbility = "behavior_ability"

func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

func BehaviorAbilityChannelModelFamilies() []binding.Kind {
	return []binding.Kind{binding.KindBehavioralRating, binding.KindCognitive}
}

func IsBehaviorAbilityChannelFamily(kind binding.Kind) bool {
	switch kind {
	case binding.KindBehavioralRating, binding.KindCognitive:
		return true
	default:
		return false
	}
}

func ResolveBehaviorAbilityChannelFamily(filter string) (binding.Kind, bool) {
	switch binding.Kind(filter) {
	case binding.KindBehavioralRating, binding.KindCognitive:
		return binding.Kind(filter), true
	default:
		return "", false
	}
}
