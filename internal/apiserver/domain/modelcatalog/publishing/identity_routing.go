package publishing

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// ProductChannelForIdentity resolves the product channel from an explicit snapshot value or kind fallback.
func ProductChannelForIdentity(kind binding.Kind, explicitChannel string) string {
	if explicitChannel != "" {
		return explicitChannel
	}
	if kind == "" {
		return ""
	}
	return string(binding.DefaultProductChannelFor(kind))
}

// AlgorithmFamilyStringFromIdentity derives the algorithm family string from model identity fields.
func AlgorithmFamilyStringFromIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) string {
	if kind == "" {
		return ""
	}
	family, ok := AlgorithmFamilyFromIdentity(kind, subKind, algorithm)
	if !ok {
		return ""
	}
	return string(family)
}
