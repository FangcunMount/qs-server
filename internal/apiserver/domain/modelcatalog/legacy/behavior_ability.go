package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

const (
	// APIKindBehaviorAbility is the external model-catalog API kind for behavior-ability models.
	// Deprecated: product channel / legacy API filter; new models use behavioral_rating or cognitive.
	APIKindBehaviorAbility = "behavior_ability"

	// PayloadFormatBehaviorAbilityScaleV1 is the published payload envelope for behavior_ability models.
	// These assets execute via legacy scale binding (MedicalScaleID), not the behavioral_rating runtime.
	PayloadFormatBehaviorAbilityScaleV1 = "assessmentmodel.behavior_ability.scale.v1"
)

// IsBehaviorAbilityScaleAdapter reports whether kind is the behavior_ability API family
// that routes through scale legacy binding instead of the behavioral_rating runtime.
// Deprecated: behavior_ability is a product channel slot, not a model family.
func IsBehaviorAbilityScaleAdapter(kind identity.Kind) bool {
	return kind == BehaviorAbilityKind()
}
