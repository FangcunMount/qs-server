package registry

import (
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type (
	// TypologyRegistry 解析类型学 运行时 modules 按 算法别名。
	TypologyRegistry = factorclassification.ModuleRegistry
	// TypologyModule 描述内置 类型学算法 别名 entry。
	TypologyModule = factorclassification.Module
	// TypologyRuntimeOptions 注入类型学 结果/report 注册表 用于 装配。
	TypologyRuntimeOptions = factorclassification.PersonalityRuntimeOptions
	// TypologyExecutor 执行配置化 类型学 评估s。
	TypologyExecutor = factorclassification.Executor
)

// 默认TypologyModules 返回内置 类型学算法 别名。
func DefaultTypologyModules() []TypologyModule {
	return factorclassification.DefaultModules()
}

// 默认TypologyRegistry 构建类型学 运行时 注册表 用于 评估 装配。
func DefaultTypologyRegistry() (TypologyRegistry, error) {
	return factorclassification.DefaultModuleRegistry()
}

// TypologyRegistryWith 构建类型学 module 注册表 使用 injectable 运行时 选项。
func TypologyRegistryWith(opts TypologyRuntimeOptions) (TypologyRegistry, error) {
	return factorclassification.NewModuleRegistryWith(opts, DefaultTypologyModules()...)
}

// 默认TypologyDescriptors 投影配置化 类型学描述符 用于 评估 装配。
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return factorclassification.DefaultTypologyDescriptors()
}

// NewConfiguredTypologyExecutor 返回默认 配置化 类型学 executor。
func NewConfiguredTypologyExecutor() (*TypologyExecutor, error) {
	return factorclassification.NewConfiguredTypologyExecutor()
}

// NewConfiguredReportBuilder 返回默认 配置化 类型学 报告构建器。
func NewConfiguredReportBuilder() (interpretationreporting.ReportBuilder, error) {
	return factorclassification.NewConfiguredReportBuilder()
}

// CategoryLabelFor 解析display label 用于 类型学算法。
func CategoryLabelFor(algorithm modelcatalog.Algorithm) string {
	return factorclassification.CategoryLabelFor(algorithm)
}
