package legacy

// Product-channel taxonomy only (§20.4): behavior_ability aggregates behavioral_rating + cognitive
// for list/options APIs. Must not drive evaluation/pipeline execution routing.

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// IsBehaviorAbilityProductChannelAPIKind 报告是否 api类型 是 behavior-ability 产品通道。
func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

// BehaviorAbilityChannelModelFamilies 返回可执行 模型家族 aggregated 按 channel。
func BehaviorAbilityChannelModelFamilies() []binding.Kind {
	return []binding.Kind{binding.KindBehavioralRating, binding.KindCognitive}
}

// IsBehaviorAbilityChannelFamily 报告是否 类型 是 listed under behavior-ability channel。
func IsBehaviorAbilityChannelFamily(kind binding.Kind) bool {
	switch kind {
	case binding.KindBehavioralRating, binding.KindCognitive:
		return true
	default:
		return false
	}
}

// ResolveBehaviorAbilityChannelFamily 映射可选 channel filter 到 模型家族 类型。
func ResolveBehaviorAbilityChannelFamily(filter string) (binding.Kind, bool) {
	switch binding.Kind(filter) {
	case binding.KindBehavioralRating, binding.KindCognitive:
		return binding.Kind(filter), true
	case "":
		return "", false
	default:
		return "", false
	}
}
