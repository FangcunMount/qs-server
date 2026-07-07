package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModuleRegistry 解析类型学 modules 按 算法。
type ModuleRegistry struct {
	runtime PersonalityRuntimeRegistry
}

// NewModuleRegistry registers 类型学 modules 按 算法。
func NewModuleRegistry(modules ...Module) (ModuleRegistry, error) {
	return NewModuleRegistryWith(PersonalityRuntimeOptions{}, modules...)
}

// NewModuleRegistryWith registers 类型学 modules 使用 injectable adapter 注册表。
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

func (r ModuleRegistry) runnerForIdentity(key evaluation.ExecutionIdentity) (algorithmRunner, error) {
	return r.runtime.runnerForIdentity(key)
}

func (r ModuleRegistry) Algorithms() []modelcatalog.Algorithm {
	return r.runtime.Algorithms()
}

func (r ModuleRegistry) Len() int {
	return r.runtime.Len()
}
