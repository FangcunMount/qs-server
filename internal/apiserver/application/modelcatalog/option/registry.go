package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// CatalogOperations captures lifecycle guards for one API kind.
type CatalogOperations struct {
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

// Allows reports whether the operation is permitted for this API kind.
func (o CatalogOperations) Allows(op capability.CatalogOperation) bool {
	switch op {
	case capability.CatalogOpCreate:
		return o.CreateSupported
	case capability.CatalogOpList:
		return o.ListSupported
	case capability.CatalogOpUpdateBasicInfo, capability.CatalogOpDelete:
		return o.CreateSupported
	case capability.CatalogOpPublish, capability.CatalogOpUnpublish, capability.CatalogOpArchive:
		return o.PublishSupported
	case capability.CatalogOpBindQuestionnaire:
		return o.BindQuestionnaire
	case capability.CatalogOpUpdateDefinition:
		return o.DefinitionUpdateSupported
	case capability.CatalogOpPreview:
		return o.PreviewSupported
	case capability.CatalogOpQRCode:
		return o.QRCodeSupported
	default:
		return false
	}
}

// RegisteredOption is the application registry entry for one catalog API kind.
type RegisteredOption struct {
	Kind           identity.Kind
	Role           capability.CapabilityRole
	APIKind        string
	DisplayName    string
	OptionsEnabled bool
	Operations     CatalogOperations
}

// Registry resolves catalog options and operation policy by API kind.
type Registry struct {
	byAPIKind map[string]RegisteredOption
	ordered   []RegisteredOption
}

// DefaultRegistry returns the process-wide catalog option registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

var defaultRegistry = NewRegistryFromDomain()

// NewRegistryFromDomain materializes options from domain catalog defaults once at startup.
func NewRegistryFromDomain() *Registry {
	presentation := capability.DefaultCatalogOptions()
	optionsByKind := make(map[identity.Kind]capability.CatalogOption, len(presentation))
	for _, item := range presentation {
		optionsByKind[item.Kind] = item
	}
	ordered := make([]RegisteredOption, 0, len(capability.DefaultFamilyCapabilities()))
	byAPIKind := make(map[string]RegisteredOption, len(ordered))
	for _, family := range capability.DefaultFamilyCapabilities() {
		presentation, ok := optionsByKind[family.Kind]
		if !ok {
			continue
		}
		apiKind := presentation.APIKind
		if apiKind == "" {
			continue
		}
		entry := RegisteredOption{
			Kind:           family.Kind,
			Role:           family.Role,
			APIKind:        apiKind,
			DisplayName:    presentation.DisplayName,
			OptionsEnabled: presentation.OptionsEnabled,
			Operations: CatalogOperations{
				CreateSupported:           family.CreateSupported,
				ListSupported:             family.ListSupported,
				PublishSupported:          family.PublishSupported,
				BindQuestionnaire:         family.BindQuestionnaire,
				DefinitionUpdateSupported: family.DefinitionUpdateSupported,
				PreviewSupported:          presentation.PreviewSupported,
				QRCodeSupported:           presentation.QRCodeSupported,
				RuntimeExecutable:         family.RuntimeExecutable,
				ExecutionPath:             family.ExecutionPath,
			},
		}
		ordered = append(ordered, entry)
		byAPIKind[apiKind] = entry
	}
	return &Registry{byAPIKind: byAPIKind, ordered: ordered}
}

// ByAPIKind resolves one registry entry.
func (r *Registry) ByAPIKind(apiKind string) (RegisteredOption, bool) {
	if r == nil {
		return RegisteredOption{}, false
	}
	entry, ok := r.byAPIKind[apiKind]
	return entry, ok
}

// Allows reports whether an API kind supports an operation.
func (r *Registry) Allows(apiKind string, op capability.CatalogOperation) bool {
	entry, ok := r.ByAPIKind(apiKind)
	if !ok {
		return false
	}
	return entry.Operations.Allows(op)
}

// ByKind resolves one registry entry by canonical model family kind.
func (r *Registry) ByKind(kind identity.Kind) (RegisteredOption, bool) {
	if r == nil {
		return RegisteredOption{}, false
	}
	for _, entry := range r.ordered {
		if entry.Kind == kind {
			return entry, true
		}
	}
	return RegisteredOption{}, false
}

// PresentationOptions returns API-facing catalog options for model families.
func (r *Registry) PresentationOptions() []capability.CatalogOption {
	if r == nil {
		return nil
	}
	out := make([]capability.CatalogOption, 0, len(r.ordered))
	for _, entry := range r.ordered {
		if entry.Role == capability.CapabilityRoleProductChannel {
			continue
		}
		out = append(out, capability.CatalogOption{
			Kind:             entry.Kind,
			APIKind:          entry.APIKind,
			DisplayName:      entry.DisplayName,
			OptionsEnabled:   entry.OptionsEnabled,
			PreviewSupported: entry.Operations.PreviewSupported,
			QRCodeSupported:  entry.Operations.QRCodeSupported,
		})
	}
	return out
}

// IsProductChannel reports whether the API kind is a product aggregation slot.
func (o RegisteredOption) IsProductChannel() bool {
	return o.Role == capability.CapabilityRoleProductChannel
}

// ProductChannelKind returns the domain kind when the entry is a product channel.
func (o RegisteredOption) ProductChannelKind() identity.Kind {
	if o.IsProductChannel() {
		return o.Kind
	}
	return ""
}
