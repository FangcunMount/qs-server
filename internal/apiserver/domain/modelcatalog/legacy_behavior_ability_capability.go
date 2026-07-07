package modelcatalog

// behaviorAbilityLegacyCapability is the legacy product-channel slot for behavior_ability API kind.
// ProductChannel is a taxonomy field, not a model family; this entry exists for legacy API compatibility only.
// It is exposed only through DefaultCapabilities() and application ModelCatalogOptions(); runtime wiring
// must read ModelFamilyCapabilitiesV2() instead.
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
