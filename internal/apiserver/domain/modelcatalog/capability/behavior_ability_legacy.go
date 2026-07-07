package capability

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// behaviorAbilityLegacyCapability is the legacy product-channel slot for behavior_ability API kind.
// ProductChannel is a taxonomy field, not a model family; this entry exists for legacy API compatibility only.
// It is exposed only through DefaultCapabilities() and application ModelCatalogOptions(); runtime wiring
// must read ModelFamilyCapabilities() instead.
func behaviorAbilityLegacyCapability() KindCapability {
	return KindCapability{
		Kind:                      legacy.BehaviorAbilityKind(),
		Role:                      CapabilityRoleProductChannel,
		APIKind:                   legacy.APIKindBehaviorAbility,
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
		ExecutionPath:             routing.ExecutionPathBehaviorAbilityScaleAdapter,
	}
}
