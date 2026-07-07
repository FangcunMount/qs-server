package capability

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

// KindCapability 是composed 能力 视图 用于 迁移。
// Deprecated: 应用代码应使用 application/modelcatalog/option.Registry。
// Prefer 模型家族能力 用于 领域 守卫 和 目录选项 用于 API surfaces。
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

// CanExecute 报告是否 模型家族 can run 通过 评估执行time。
func (c KindCapability) CanExecute() bool {
	return c.RuntimeExecutable
}

// 默认Capabilities 返回composed 能力 矩阵。
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

// ModelFamilyCapabilities 返回可执行 model-家族 能力 仅。
func ModelFamilyCapabilities() []KindCapability {
	out := make([]KindCapability, 0)
	for _, cap := range DefaultCapabilities() {
		if cap.Role == CapabilityRoleModelFamily {
			out = append(out, cap)
		}
	}
	return out
}

// ModelFamilyCapabilityByKind 解析model-家族 能力, excluding 产品通道。
func ModelFamilyCapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	family, ok := FamilyCapabilityByKind(kind)
	if !ok || family.Role != CapabilityRoleModelFamily {
		return KindCapability{}, false
	}
	option, _ := CatalogOptionByKind(kind)
	return mergeKindCapability(family, option), true
}

// CapabilityByKind 解析能力 用于 规范 领域类型。
func CapabilityByKind(kind identity.Kind) (KindCapability, bool) {
	family, ok := FamilyCapabilityByKind(kind)
	if !ok {
		return KindCapability{}, false
	}
	option, _ := CatalogOptionByKind(kind)
	return mergeKindCapability(family, option), true
}

// CapabilityByAPIKind 解析能力 using 外部 model-目录 API 类型。
func CapabilityByAPIKind(apiKind string) (KindCapability, bool) {
	for _, option := range defaultCatalogOptions {
		if option.APIKind == apiKind {
			return CapabilityByKind(option.Kind)
		}
	}
	return KindCapability{}, false
}

// RuntimeExecutableKinds 返回领域类型 that have direct 评估 描述符。
func RuntimeExecutableKinds() []identity.Kind {
	out := make([]identity.Kind, 0)
	for _, cap := range DefaultFamilyCapabilities() {
		if cap.RuntimeExecutable {
			out = append(out, cap.Kind)
		}
	}
	return out
}
