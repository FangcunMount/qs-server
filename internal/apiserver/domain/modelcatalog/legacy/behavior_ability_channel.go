package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// IsBehaviorAbilityProductChannelAPIKind 报告是否 api类型 是 behavior-ability 产品通道。
func IsBehaviorAbilityProductChannelAPIKind(apiKind string) bool {
	return apiKind == APIKindBehaviorAbility
}

// BehaviorAbilityChannelModelFamilies 返回可执行 模型家族 aggregated 按 channel。
func BehaviorAbilityChannelModelFamilies() []identity.Kind {
	return []identity.Kind{identity.KindBehavioralRating, identity.KindCognitive}
}

// IsBehaviorAbilityChannelFamily 报告是否 类型 是 listed under behavior-ability channel。
func IsBehaviorAbilityChannelFamily(kind identity.Kind) bool {
	switch kind {
	case identity.KindBehavioralRating, identity.KindCognitive:
		return true
	default:
		return false
	}
}

// ResolveBehaviorAbilityChannelFamily 映射可选 channel filter 到 模型家族 类型。
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
