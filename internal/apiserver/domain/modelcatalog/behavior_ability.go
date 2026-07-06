package modelcatalog

const (
	// APIKindBehaviorAbility is the external model-catalog API kind for behavior-ability models.
	APIKindBehaviorAbility = "behavior_ability"

	// PayloadFormatBehaviorAbilityScaleV1 is the published payload envelope for behavior_ability models.
	// These assets execute via legacy scale binding (MedicalScaleID), not a standalone
	// behavioral_rating runtime descriptor.
	PayloadFormatBehaviorAbilityScaleV1 = "assessmentmodel.behavior_ability.scale.v1"
)

// IsBehaviorAbilityScaleAdapter reports whether kind is the behavior_ability API family
// that routes through scale legacy binding instead of a behavioral_rating runtime.
func IsBehaviorAbilityScaleAdapter(kind Kind) bool {
	return kind == KindBehavioralRating
}

// BehaviorAbilityScaleAdapterCapability returns the canonical capability entry for the
// behavior_ability API family.
func BehaviorAbilityScaleAdapterCapability() (KindCapability, bool) {
	return CapabilityByKind(KindBehavioralRating)
}
