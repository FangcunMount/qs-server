package modelcatalog

// KindCapability is the canonical capability matrix for a model family.
// API options, create/publish guards, and runtime descriptor export should read this table.
type KindCapability struct {
	Kind                      Kind
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
	RuntimeViaScaleLegacy     bool
	ExecutionPath             ExecutionPath
}

func (c KindCapability) CanExecute() bool {
	return c.RuntimeExecutable || c.RuntimeViaScaleLegacy
}

var defaultCapabilities = []KindCapability{
	{
		Kind:                      KindPersonality,
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
		ExecutionPath:             ExecutionPathTypologyDescriptor,
	},
	{
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
	},
	{
		Kind:                      KindBehavioralRating,
		Role:                      CapabilityRoleModelFamily,
		APIKind:                   string(KindBehavioralRating),
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
		ExecutionPath:             ExecutionPathBehavioralRatingDescriptor,
	},
	{
		Kind:              KindScale,
		Role:              CapabilityRoleModelFamily,
		APIKind:           "medical_scale",
		DisplayName:       "医学量表",
		OptionsEnabled:    true,
		ListSupported:     false,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathScaleDescriptor,
	},
	{
		Kind:                      KindCognitive,
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
		ExecutionPath:             ExecutionPathCognitiveDescriptor,
	},
	{
		Kind:           KindCustom,
		Role:           CapabilityRoleModelFamily,
		APIKind:        "custom",
		DisplayName:    "自定义测评",
		OptionsEnabled: false,
		ExecutionPath:  ExecutionPathNone,
	},
}

// DefaultCapabilities returns the built-in model-catalog capability matrix.
func DefaultCapabilities() []KindCapability {
	out := make([]KindCapability, len(defaultCapabilities))
	copy(out, defaultCapabilities)
	return out
}

// CapabilityByKind resolves capability for a canonical domain kind.
func CapabilityByKind(kind Kind) (KindCapability, bool) {
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
func RuntimeExecutableKinds() []Kind {
	out := make([]Kind, 0, len(defaultCapabilities))
	for _, cap := range defaultCapabilities {
		if cap.RuntimeExecutable {
			out = append(out, cap.Kind)
		}
	}
	return out
}
