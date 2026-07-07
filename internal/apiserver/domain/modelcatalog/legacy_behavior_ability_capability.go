package modelcatalog

// behaviorAbilityLegacyCapability is the legacy product-channel slot for behavior_ability API kind.
// ProductChannel is a taxonomy field, not a model family; this entry exists for legacy API compatibility only.
// It must not receive new draft models; listings aggregate behavioral_rating and cognitive families.
func behaviorAbilityLegacyCapability() KindCapability {
	return KindCapability{
		Kind:                      KindBehaviorAbility,
		Role:                      CapabilityRoleProductChannel,
		APIKind:                   APIKindBehaviorAbility,
		DisplayName:               "行为能力测评",
		OptionsEnabled:            true,
		CreateSupported:           false,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		PreviewSupported:          false,
		QRCodeSupported:           true,
		RuntimeViaScaleLegacy:     true,
		ExecutionPath:             ExecutionPathBehaviorAbilityScaleAdapter,
	}
}
