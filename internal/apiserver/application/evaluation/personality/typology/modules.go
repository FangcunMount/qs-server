package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology/legacy"
)

// DefaultModules returns algorithm alias entries for the configured typology runtime.
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

// AllModules returns all built-in typology modules.
func AllModules() []Module {
	return DefaultModules()
}

// DefaultAlgorithms returns algorithms registered by DefaultModules.
func DefaultAlgorithms() []assessmentmodel.Algorithm {
	return typologylegacy.DefaultAlgorithmAliases()
}

// CategoryLabelFor resolves the display label for a typology algorithm.
func CategoryLabelFor(algorithm assessmentmodel.Algorithm) string {
	return typologylegacy.CategoryLabelFor(algorithm)
}

// DefaultModuleRegistry builds the default typology module registry.
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
