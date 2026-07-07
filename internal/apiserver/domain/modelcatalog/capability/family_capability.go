package capability

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// ModelFamilyCapability 记录领域 execution 和 lifecycle 守卫 用于 模型家族。
type ModelFamilyCapability struct {
	Kind                      identity.Kind
	Role                      CapabilityRole
	CreateSupported           bool
	ListSupported             bool
	PublishSupported          bool
	BindQuestionnaire         bool
	DefinitionUpdateSupported bool
	RuntimeExecutable         bool
	ExecutionPath             routing.ExecutionPath
}

func (c ModelFamilyCapability) CanExecute() bool {
	return c.RuntimeExecutable
}

func (c ModelFamilyCapability) IsProductChannel() bool {
	return c.Role == CapabilityRoleProductChannel
}

func (c ModelFamilyCapability) AllowsNewDraft() bool {
	return c.CreateSupported
}

var defaultFamilyCapabilities = []ModelFamilyCapability{
	{
		Kind:                      identity.KindPersonality,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathTypologyDescriptor,
	},
	{
		Kind:                      identity.KindBehavioralRating,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathBehavioralRatingDescriptor,
	},
	{
		Kind:              identity.KindScale,
		Role:              CapabilityRoleModelFamily,
		ListSupported:     false,
		RuntimeExecutable: true,
		ExecutionPath:     routing.ExecutionPathScaleDescriptor,
	},
	{
		Kind:                      identity.KindCognitive,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathCognitiveDescriptor,
	},
	{
		Kind:          identity.KindCustom,
		Role:          CapabilityRoleModelFamily,
		ExecutionPath: routing.ExecutionPathNone,
	},
}

// 默认FamilyCapabilities 返回领域-仅 模型家族 能力。
func DefaultFamilyCapabilities() []ModelFamilyCapability {
	out := make([]ModelFamilyCapability, len(defaultFamilyCapabilities))
	copy(out, defaultFamilyCapabilities)
	return out
}

// FamilyCapabilityByKind 解析model-家族 能力。
func FamilyCapabilityByKind(kind identity.Kind) (ModelFamilyCapability, bool) {
	for _, cap := range defaultFamilyCapabilities {
		if cap.Kind == kind {
			return cap, true
		}
	}
	return ModelFamilyCapability{}, false
}
