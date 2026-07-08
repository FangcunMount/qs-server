package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
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

var defaultRegistry = NewRegistry()

// NewRegistry 物化应用层目录选项注册表。
func NewRegistry() *Registry {
	ordered := make([]RegisteredOption, len(defaultRegisteredOptions))
	copy(ordered, defaultRegisteredOptions)
	byAPIKind := make(map[string]RegisteredOption, len(ordered))
	for _, entry := range ordered {
		if entry.APIKind == "" {
			continue
		}
		byAPIKind[entry.APIKind] = entry
	}
	return &Registry{byAPIKind: byAPIKind, ordered: ordered}
}

// NewRegistryFromDomain 保留旧测试/调用方名称。
//
// Deprecated: 使用 NewRegistry；目录展示元数据由 application/option 拥有。
func NewRegistryFromDomain() *Registry {
	return NewRegistry()
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
func (r *Registry) PresentationOptions() []ModelCatalogOption {
	if r == nil {
		return nil
	}
	out := make([]ModelCatalogOption, 0, len(r.ordered))
	for _, entry := range r.ordered {
		if entry.Role == capability.CapabilityRoleProductChannel {
			continue
		}
		out = append(out, entry.catalogOption())
	}
	return out
}

// RegisteredOptions returns all registry entries in registration order.
func (r *Registry) RegisteredOptions() []RegisteredOption {
	if r == nil {
		return nil
	}
	out := make([]RegisteredOption, len(r.ordered))
	copy(out, r.ordered)
	return out
}

func (o RegisteredOption) catalogOption() ModelCatalogOption {
	return ModelCatalogOption{
		Kind:             o.Kind,
		APIKind:          o.APIKind,
		DisplayName:      o.DisplayName,
		OptionsEnabled:   o.OptionsEnabled,
		PreviewSupported: o.Operations.PreviewSupported,
		QRCodeSupported:  o.Operations.QRCodeSupported,
	}
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

var defaultRegisteredOptions = []RegisteredOption{
	{
		Kind:           identity.KindPersonality,
		Role:           capability.CapabilityRoleModelFamily,
		APIKind:        "personality",
		DisplayName:    "人格测评",
		OptionsEnabled: true,
		Operations: CatalogOperations{
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
	},
	{
		Kind:           identity.KindBehavioralRating,
		Role:           capability.CapabilityRoleModelFamily,
		APIKind:        string(identity.KindBehavioralRating),
		DisplayName:    "行为评分",
		OptionsEnabled: true,
		Operations: CatalogOperations{
			CreateSupported:           true,
			ListSupported:             true,
			PublishSupported:          true,
			BindQuestionnaire:         true,
			DefinitionUpdateSupported: true,
			QRCodeSupported:           true,
			RuntimeExecutable:         true,
			ExecutionPath:             routing.ExecutionPathBehavioralRatingDescriptor,
		},
	},
	{
		Kind:           identity.KindScale,
		Role:           capability.CapabilityRoleModelFamily,
		APIKind:        "medical_scale",
		DisplayName:    "医学量表",
		OptionsEnabled: true,
		Operations: CatalogOperations{
			RuntimeExecutable: true,
			ExecutionPath:     routing.ExecutionPathScaleDescriptor,
		},
	},
	{
		Kind:           identity.KindCognitive,
		Role:           capability.CapabilityRoleModelFamily,
		APIKind:        "cognitive",
		DisplayName:    "认知测评",
		OptionsEnabled: true,
		Operations: CatalogOperations{
			CreateSupported:           true,
			ListSupported:             true,
			PublishSupported:          true,
			BindQuestionnaire:         true,
			DefinitionUpdateSupported: true,
			QRCodeSupported:           true,
			RuntimeExecutable:         true,
			ExecutionPath:             routing.ExecutionPathCognitiveDescriptor,
		},
	},
	{
		Kind:        identity.KindCustom,
		Role:        capability.CapabilityRoleModelFamily,
		APIKind:     "custom",
		DisplayName: "自定义测评",
		Operations: CatalogOperations{
			ExecutionPath: routing.ExecutionPathNone,
		},
	},
	{
		Kind:        identity.Kind(legacy.APIKindBehaviorAbility),
		Role:        capability.CapabilityRoleProductChannel,
		APIKind:     legacy.APIKindBehaviorAbility,
		DisplayName: "行为能力",
		Operations: CatalogOperations{
			ListSupported:   false,
			CreateSupported: false,
		},
	},
}
