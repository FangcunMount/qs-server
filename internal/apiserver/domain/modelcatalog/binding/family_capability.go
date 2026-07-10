package binding

// ModelFamilyCapability 记录领域 execution 和 lifecycle 守卫 用于 模型家族。
type ModelFamilyCapability struct {
	Kind                      Kind
	Role                      CapabilityRole
	CreateSupported           bool
	ListSupported             bool
	PublishSupported          bool
	BindQuestionnaire         bool
	DefinitionUpdateSupported bool
	RuntimeExecutable         bool
	ExecutionPath             ExecutionPath
}

// KindCapability is the mechanism-oriented alias for model-family capability guards.
type KindCapability = ModelFamilyCapability

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
		Kind:                      KindTypology,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             ExecutionPathTypologyDescriptor,
	},
	{
		Kind:                      KindBehavioralRating,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             ExecutionPathBehavioralRatingDescriptor,
	},
	{
		Kind:              KindScale,
		Role:              CapabilityRoleModelFamily,
		ListSupported:     false,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathScaleDescriptor,
	},
	{
		Kind:                      KindCognitive,
		Role:                      CapabilityRoleModelFamily,
		CreateSupported:           true,
		ListSupported:             true,
		PublishSupported:          true,
		BindQuestionnaire:         true,
		DefinitionUpdateSupported: true,
		RuntimeExecutable:         true,
		ExecutionPath:             ExecutionPathCognitiveDescriptor,
	},
}

// FamilyCapabilityByKind resolves model-family capability guards.
func FamilyCapabilityByKind(kind Kind) (ModelFamilyCapability, bool) {
	for _, cap := range defaultFamilyCapabilities {
		if cap.Kind == kind {
			return cap, true
		}
	}
	return ModelFamilyCapability{}, false
}

// RuntimeExecutableKinds returns domain types that have direct evaluation descriptors.
func RuntimeExecutableKinds() []Kind {
	out := make([]Kind, 0)
	for _, cap := range defaultFamilyCapabilities {
		if cap.RuntimeExecutable {
			out = append(out, cap.Kind)
		}
	}
	return out
}
