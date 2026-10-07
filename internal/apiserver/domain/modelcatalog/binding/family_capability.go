package binding

// ModelFamilyCapability records runtime execution guards for a model family.
type ModelFamilyCapability struct {
	Kind              Kind
	RuntimeExecutable bool
	ExecutionPath     ExecutionPath
}

var defaultFamilyCapabilities = []ModelFamilyCapability{
	{
		Kind:              KindTypology,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathTypologyDescriptor,
	},
	{
		Kind:              KindBehavioralRating,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathBehavioralRatingDescriptor,
	},
	{
		Kind:              KindScale,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathScaleDescriptor,
	},
	{
		Kind:              KindCognitive,
		RuntimeExecutable: true,
		ExecutionPath:     ExecutionPathCognitiveDescriptor,
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
