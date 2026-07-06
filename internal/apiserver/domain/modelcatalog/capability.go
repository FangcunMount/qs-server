package modelcatalog

// KindCapability is the canonical capability matrix for a model family.
// API options, create/publish guards, and runtime descriptor export should read this table.
type KindCapability struct {
	Kind                  Kind
	APIKind               string
	DisplayName           string
	OptionsEnabled        bool
	CreateSupported       bool
	ListSupported         bool
	PublishSupported      bool
	BindQuestionnaire     bool
	PreviewSupported      bool
	QRCodeSupported       bool
	RuntimeExecutable     bool
	RuntimeViaScaleLegacy bool
	ExecutionPath         string
}

func (c KindCapability) CanExecute() bool {
	return c.RuntimeExecutable || c.RuntimeViaScaleLegacy
}

var defaultCapabilities = []KindCapability{
	{
		Kind:              KindPersonality,
		APIKind:           "personality",
		DisplayName:       "人格测评",
		OptionsEnabled:    true,
		CreateSupported:   true,
		ListSupported:     true,
		PublishSupported:  true,
		BindQuestionnaire: true,
		PreviewSupported:  true,
		QRCodeSupported:   true,
		RuntimeExecutable: true,
		ExecutionPath:     "typology_descriptor",
	},
	{
		Kind:                  KindBehavioralRating,
		APIKind:               APIKindBehaviorAbility,
		DisplayName:           "行为能力测评",
		OptionsEnabled:        true,
		CreateSupported:       true,
		ListSupported:         true,
		PublishSupported:      true,
		BindQuestionnaire:     true,
		PreviewSupported:      false,
		QRCodeSupported:       true,
		RuntimeViaScaleLegacy: true,
		ExecutionPath:         "behavior_ability_scale_adapter",
	},
	{
		Kind:              KindScale,
		APIKind:           "medical_scale",
		DisplayName:       "医学量表",
		OptionsEnabled:    true,
		ListSupported:     false,
		RuntimeExecutable: true,
		ExecutionPath:     "scale_descriptor",
	},
	{
		Kind:           KindCognitive,
		APIKind:        "cognitive",
		DisplayName:    "认知测评",
		OptionsEnabled: false,
		ExecutionPath:  "none",
	},
	{
		Kind:           KindCustom,
		APIKind:        "custom",
		DisplayName:    "自定义测评",
		OptionsEnabled: false,
		ExecutionPath:  "none",
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
