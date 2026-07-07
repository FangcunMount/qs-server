package capability

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// KindCapability is the canonical capability matrix for a model family.
// API options, create/publish guards, and runtime descriptor export should read this table.
// ProductChannel is a taxonomy field on AssessmentModel, not a model family capability.
// Use ModelFamilyCapabilities for runtime/create/publish guards on executable families.
type KindCapability struct {
	Kind                      identity.Kind
	Role                      CapabilityRole
	APIKind                   string
	DisplayName               string
	OptionsEnabled            bool
	CreateSupported           bool
	ListSupported             bool
	PublishSupported          bool
	BindQuestionnaire         bool
	DefinitionUpdateSupported bool
	PreviewSupported          bool
	QRCodeSupported           bool
	RuntimeExecutable         bool
	ExecutionPath             routing.ExecutionPath
}

func (c KindCapability) CanExecute() bool {
	return c.RuntimeExecutable
}

var defaultCapabilities = []KindCapability{
	{
		Kind:                      identity.KindPersonality,
		Role:                      CapabilityRoleModelFamily,
		APIKind:                   "personality",
		DisplayName:               "人格测评",
		OptionsEnabled:            true,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		PreviewSupported:          true,
		QRCodeSupported:           true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathTypologyDescriptor,
	},
	{
		Kind:                      identity.KindBehavioralRating,
		Role:                      CapabilityRoleModelFamily,
		APIKind:                   string(identity.KindBehavioralRating),
		DisplayName:               "行为评分",
		OptionsEnabled:            true,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		PreviewSupported:          false,
		QRCodeSupported:           true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathBehavioralRatingDescriptor,
	},
	{
		Kind:              identity.KindScale,
		Role:              CapabilityRoleModelFamily,
		APIKind:           "medical_scale",
		DisplayName:       "医学量表",
		OptionsEnabled:    true,
		ListSupported:     false,
		RuntimeExecutable: true,
		ExecutionPath:     routing.ExecutionPathScaleDescriptor,
	},
	{
		Kind:                      identity.KindCognitive,
		Role:                      CapabilityRoleModelFamily,
		APIKind:                   "cognitive",
		DisplayName:               "认知测评",
		OptionsEnabled:            true,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		PreviewSupported:          false,
		QRCodeSupported:           true,
		RuntimeExecutable:         true,
		ExecutionPath:             routing.ExecutionPathCognitiveDescriptor,
	},
	{
		Kind:           identity.KindCustom,
		Role:           CapabilityRoleModelFamily,
		APIKind:        "custom",
		DisplayName:    "自定义测评",
		OptionsEnabled: false,
		ExecutionPath:  routing.ExecutionPathNone,
	},
}

// DefaultCapabilities returns the built-in model-catalog capability matrix.
func DefaultCapabilities() []KindCapability {
	out := make([]KindCapability, len(defaultCapabilities))
	copy(out, defaultCapabilities)
	return out
}

// ModelFamilyCapabilities returns executable model-family capabilities only.
// Product-channel slots such as behavior_ability are excluded.
func ModelFamilyCapabilities() []KindCapability {
	out := make([]KindCapability, 0, len(defaultCapabilities))
	for _, cap := range defaultCapabilities {
		if cap.Role == CapabilityRoleModelFamily {
			out = append(out, cap)
		}
	}
	return out
}

// ModelFamilyCapabilityByKind resolves a model-family capability, excluding product channels.
func ModelFamilyCapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	cap, ok := CapabilityByKind(kind)
	if !ok || cap.Role != CapabilityRoleModelFamily {
		return KindCapability{}, false
	}
	return cap, true
}

// CapabilityByKind resolves capability for a canonical domain kind.
func CapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	for _, cap := range defaultCapabilities {
		if cap.Kind == kind {
			return cap, true
		}
	}
	return KindCapability{}, false
}

// CapabilityByAPIKind resolves capability using the external model-catalog API kind.
func CapabilityByAPIKind(apiKind string) (KindCapability, bool) {
	for _, cap := range defaultCapabilities {
		if cap.APIKind == apiKind {
			return cap, true
		}
	}
	return KindCapability{}, false
}

// RuntimeExecutableKinds returns domain kinds that have a direct evaluation descriptor.
func RuntimeExecutableKinds() []identity.Kind {
	out := make([]identity.Kind, 0, len(defaultCapabilities))
	for _, cap := range ModelFamilyCapabilities() {
		if cap.RuntimeExecutable {
			out = append(out, cap.Kind)
		}
	}
	return out
}
