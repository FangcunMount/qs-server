package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModuleRegistry resolves typology modules by algorithm.
type ModuleRegistry struct {
	runtime PersonalityRuntimeRegistry
}

// NewModuleRegistry registers typology modules by algorithm.
func NewModuleRegistry(modules ...Module) (ModuleRegistry, error) {
	return NewModuleRegistryWith(PersonalityRuntimeOptions{}, modules...)
}

// NewModuleRegistryWith registers typology modules with injectable adapter registries.
func NewModuleRegistryWith(opts PersonalityRuntimeOptions, modules ...Module) (ModuleRegistry, error) {
	if len(modules) == 0 {
		return ModuleRegistry{}, fmt.Errorf("typology modules are required")
	}
	algorithms := make([]modelcatalog.Algorithm, 0, len(modules))
	for _, module := range modules {
		if module.Algorithm == "" {
			return ModuleRegistry{}, fmt.Errorf("typology module algorithm is required")
		}
		algorithms = append(algorithms, module.Algorithm)
	}
	return ModuleRegistry{runtime: NewPersonalityRuntimeRegistryWith(opts, algorithms...)}, nil
}

func (r ModuleRegistry) runnerFor(algorithm modelcatalog.Algorithm) (algorithmRunner, error) {
	return r.runtime.runnerFor(algorithm)
}

func (r ModuleRegistry) runnerForKey(key evaluation.EvaluatorKey) (algorithmRunner, error) {
	return r.runtime.runnerForKey(key)
}

func (r ModuleRegistry) Algorithms() []modelcatalog.Algorithm {
	return r.runtime.Algorithms()
}

func (r ModuleRegistry) Len() int {
	return r.runtime.Len()
}
