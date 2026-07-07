package modelcatalog

// behaviorAbilityLegacyCapability is the legacy product-channel slot for behavior_ability API kind.
// It must not receive new draft models; listings aggregate behavioral_rating and cognitive families.
// Kept for legacy scale-adapter read paths and API compatibility only.
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
