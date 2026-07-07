package capability

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// KindCapability is the composed capability view used during migration.
// Deprecated: application code should use application/modelcatalog/option.Registry.
// Prefer ModelFamilyCapability for domain guards and CatalogOption for API surfaces.
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

// CanExecute reports whether the model family can run through evaluation runtime.
func (c KindCapability) CanExecute() bool {
	return c.RuntimeExecutable
}

// DefaultCapabilities returns the composed capability matrix.
func DefaultCapabilities() []KindCapability {
	families := DefaultFamilyCapabilities()
	optionsByKind := make(map[identity.Kind]CatalogOption, len(defaultCatalogOptions))
	for _, option := range defaultCatalogOptions {
		optionsByKind[option.Kind] = option
	}
	out := make([]KindCapability, 0, len(families))
	for _, family := range families {
		out = append(out, mergeKindCapability(family, optionsByKind[family.Kind]))
	}
	return out
}

// ModelFamilyCapabilities returns executable model-family capabilities only.
func ModelFamilyCapabilities() []KindCapability {
	out := make([]KindCapability, 0)
	for _, cap := range DefaultCapabilities() {
		if cap.Role == CapabilityRoleModelFamily {
			out = append(out, cap)
		}
	}
	return out
}

// ModelFamilyCapabilityByKind resolves a model-family capability, excluding product channels.
func ModelFamilyCapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	family, ok := FamilyCapabilityByKind(kind)
	if !ok || family.Role != CapabilityRoleModelFamily {
		return KindCapability{}, false
	}
	option, _ := CatalogOptionByKind(kind)
	return mergeKindCapability(family, option), true
}

// CapabilityByKind resolves capability for a canonical domain kind.
func CapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	family, ok := FamilyCapabilityByKind(kind)
	if !ok {
		return KindCapability{}, false
	}
	option, _ := CatalogOptionByKind(kind)
	return mergeKindCapability(family, option), true
}

// CapabilityByAPIKind resolves capability using the external model-catalog API kind.
func CapabilityByAPIKind(apiKind string) (KindCapability, bool) {
	for _, option := range defaultCatalogOptions {
		if option.APIKind == apiKind {
			return CapabilityByKind(option.Kind)
		}
	}
	return KindCapability{}, false
}

// RuntimeExecutableKinds returns domain kinds that have a direct evaluation descriptor.
func RuntimeExecutableKinds() []identity.Kind {
	out := make([]identity.Kind, 0)
	for _, cap := range DefaultFamilyCapabilities() {
		if cap.RuntimeExecutable {
			out = append(out, cap.Kind)
		}
	}
	return out
}
