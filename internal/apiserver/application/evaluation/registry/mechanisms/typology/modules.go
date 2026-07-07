package typology

import (
	"fmt"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// 默认Modules 返回算法别名 entries 用于 配置化 类型学 运行时。
func DefaultModules() []Module {
	modules := make([]Module, 0, len(typologylegacy.DefaultAlgorithmAliases()))
	for _, algorithm := range typologylegacy.DefaultAlgorithmAliases() {
		modules = append(modules, Module{
			Algorithm:     algorithm,
			CategoryLabel: typologylegacy.CategoryLabelFor(algorithm),
		})
	}
	return modules
}

// AllModules 返回全部内置 类型学 modules。
func AllModules() []Module {
	return DefaultModules()
}

// 默认Algorithms 返回算法s 已注册 按 默认Modules。
func DefaultAlgorithms() []modelcatalog.Algorithm {
	return typologylegacy.DefaultAlgorithmAliases()
}

// CategoryLabelFor 解析display label 用于 类型学算法。
func CategoryLabelFor(algorithm modelcatalog.Algorithm) string {
	return typologylegacy.CategoryLabelFor(algorithm)
}

// 默认ModuleRegistry 构建默认 类型学 module 注册表。
func DefaultModuleRegistry() (ModuleRegistry, error) {
	return DefaultPersonalityRuntimeRegistry().AsModuleRegistry(), nil
}

func mustDefaultModuleRegistry() ModuleRegistry {
	registry, err := DefaultModuleRegistry()
	if err != nil {
		panic(fmt.Sprintf("default typology module registry: %v", err))
	}
	return registry
}
