package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// CatalogOperations 记录lifecycle 守卫 用于 一个API 类型。
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

// Allows 报告是否 操作 是 permitted 用于 这个API 类型。
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

// RegisteredOption 是应用层注册表条目 用于 一个目录 API 类型。
type RegisteredOption struct {
	Kind           identity.Kind
	Role           capability.CapabilityRole
	APIKind        string
	DisplayName    string
	OptionsEnabled bool
	Operations     CatalogOperations
}

// Registry 解析目录选项 和 操作策略 按 API 类型。
type Registry struct {
	byAPIKind map[string]RegisteredOption
	ordered   []RegisteredOption
}

// 默认Registry 返回进程-wide 目录选项注册表。
func DefaultRegistry() *Registry {
	return defaultRegistry
}

var defaultRegistry = NewRegistryFromDomain()

// NewRegistryFromDomain 物化选项 从 领域 目录默认值 在启动时一次性。
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

// ByAPIKind 解析一个注册表条目。
func (r *Registry) ByAPIKind(apiKind string) (RegisteredOption, bool) {
	if r == nil {
		return RegisteredOption{}, false
	}
	entry, ok := r.byAPIKind[apiKind]
	return entry, ok
}

// Allows 报告是否 API 类型 supports 操作。
func (r *Registry) Allows(apiKind string, op capability.CatalogOperation) bool {
	entry, ok := r.ByAPIKind(apiKind)
	if !ok {
		return false
	}
	return entry.Operations.Allows(op)
}

// ByKind 解析一个注册表条目 按 规范模型家族类型。
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

// PresentationOptions 返回API-facing 目录选项 用于 模型家族。
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

// IsProductChannel 报告是否 API 类型 是 产品聚合槽位。
func (o RegisteredOption) IsProductChannel() bool {
	return o.Role == capability.CapabilityRoleProductChannel
}

// ProductChannelKind 返回领域类型 when entry 是 产品通道。
func (o RegisteredOption) ProductChannelKind() identity.Kind {
	if o.IsProductChannel() {
		return o.Kind
	}
	return ""
}
