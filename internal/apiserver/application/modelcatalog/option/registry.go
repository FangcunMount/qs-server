package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
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
	ExecutionPath             publishing.ExecutionPath
}

// Allows 报告是否 操作 是 permitted 用于 这个API 类型。
func (o CatalogOperations) Allows(op binding.CatalogOperation) bool {
	switch op {
	case binding.CatalogOpCreate:
		return o.CreateSupported
	case binding.CatalogOpList:
		return o.ListSupported
	case binding.CatalogOpUpdateBasicInfo, binding.CatalogOpDelete:
		return o.CreateSupported
	case binding.CatalogOpPublish, binding.CatalogOpUnpublish, binding.CatalogOpArchive:
		return o.PublishSupported
	case binding.CatalogOpBindQuestionnaire:
		return o.BindQuestionnaire
	case binding.CatalogOpUpdateDefinition:
		return o.DefinitionUpdateSupported
	case binding.CatalogOpPreview:
		return o.PreviewSupported
	case binding.CatalogOpQRCode:
		return o.QRCodeSupported
	default:
		return false
	}
}

// RegisteredOption 是应用层注册表条目 用于 一个目录 API 类型。
type RegisteredOption struct {
	Kind           binding.Kind
	Role           binding.CapabilityRole
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
func (r *Registry) Allows(apiKind string, op binding.CatalogOperation) bool {
	entry, ok := r.ByAPIKind(apiKind)
	if !ok {
		return false
	}
	return entry.Operations.Allows(op)
}

// ByKind 解析一个注册表条目 按 规范模型家族类型。
func (r *Registry) ByKind(kind binding.Kind) (RegisteredOption, bool) {
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
		if entry.Role == binding.CapabilityRoleProductChannel {
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

func catalogOperationsFromCapability(cap binding.ModelFamilyCapability) CatalogOperations {
	return CatalogOperations{
		CreateSupported:           cap.CreateSupported,
		ListSupported:             cap.ListSupported,
		PublishSupported:          cap.PublishSupported,
		BindQuestionnaire:         cap.BindQuestionnaire,
		DefinitionUpdateSupported: cap.DefinitionUpdateSupported,
		RuntimeExecutable:         cap.RuntimeExecutable,
		ExecutionPath:             cap.ExecutionPath,
	}
}

type registeredPresentation struct {
	apiKind          string
	displayName      string
	optionsEnabled   bool
	previewSupported bool
	qrCodeSupported  bool
}

func registeredModelFamily(kind binding.Kind, presentation registeredPresentation) RegisteredOption {
	cap, ok := binding.FamilyCapabilityByKind(kind)
	if !ok {
		cap = binding.ModelFamilyCapability{Kind: kind, Role: binding.CapabilityRoleModelFamily}
	}
	ops := catalogOperationsFromCapability(cap)
	ops.PreviewSupported = presentation.previewSupported
	ops.QRCodeSupported = presentation.qrCodeSupported
	return RegisteredOption{
		Kind:           kind,
		Role:           binding.CapabilityRoleModelFamily,
		APIKind:        presentation.apiKind,
		DisplayName:    presentation.displayName,
		OptionsEnabled: presentation.optionsEnabled,
		Operations:     ops,
	}
}

// IsProductChannel 报告是否 API 类型 是 产品聚合槽位。
func (o RegisteredOption) IsProductChannel() bool {
	return o.Role == binding.CapabilityRoleProductChannel
}

// ProductChannelKind 返回领域类型 when entry 是 产品通道。
func (o RegisteredOption) ProductChannelKind() binding.Kind {
	if o.IsProductChannel() {
		return o.Kind
	}
	return ""
}

var defaultRegisteredOptions = []RegisteredOption{
	registeredModelFamily(binding.KindPersonality, registeredPresentation{
		apiKind: "personality", displayName: "人格测评", optionsEnabled: true,
		previewSupported: true, qrCodeSupported: true,
	}),
	registeredModelFamily(binding.KindBehavioralRating, registeredPresentation{
		apiKind: string(binding.KindBehavioralRating), displayName: "行为评分", optionsEnabled: true,
		qrCodeSupported: true,
	}),
	registeredModelFamily(binding.KindScale, registeredPresentation{
		apiKind: "medical_scale", displayName: "医学量表", optionsEnabled: true,
	}),
	registeredModelFamily(binding.KindCognitive, registeredPresentation{
		apiKind: "cognitive", displayName: "认知测评", optionsEnabled: true,
		qrCodeSupported: true,
	}),
	registeredModelFamily(binding.KindCustom, registeredPresentation{
		apiKind: "custom", displayName: "自定义测评",
	}),
	{
		Kind:        binding.Kind(legacy.APIKindBehaviorAbility),
		Role:        binding.CapabilityRoleProductChannel,
		APIKind:     legacy.APIKindBehaviorAbility,
		DisplayName: "行为能力",
		Operations: CatalogOperations{
			ListSupported:   false,
			CreateSupported: false,
		},
	},
}
