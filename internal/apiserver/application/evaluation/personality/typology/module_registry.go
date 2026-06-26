package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

// ModuleRegistry resolves typology modules by algorithm.
type ModuleRegistry struct {
	modules map[assessmentmodel.Algorithm]Module
}

// NewModuleRegistry registers typology modules by algorithm.
func NewModuleRegistry(modules ...Module) (ModuleRegistry, error) {
	registry := ModuleRegistry{modules: make(map[assessmentmodel.Algorithm]Module, len(modules))}
	for _, module := range modules {
		if module.Algorithm == "" {
			return ModuleRegistry{}, fmt.Errorf("typology module algorithm is required")
		}
		if module.Adapter == nil {
			return ModuleRegistry{}, fmt.Errorf("typology module %s adapter is required", module.Algorithm)
		}
		if module.outcomeAssembler == nil {
			return ModuleRegistry{}, fmt.Errorf("typology module %s outcome assembler is required", module.Algorithm)
		}
		if module.reportBuilder == nil {
			return ModuleRegistry{}, fmt.Errorf("typology module %s report builder is required", module.Algorithm)
		}
		if module.Adapter.Algorithm() != module.Algorithm {
			return ModuleRegistry{}, fmt.Errorf("typology module %s adapter mismatch: %s", module.Algorithm, module.Adapter.Algorithm())
		}
		registry.modules[module.Algorithm] = module
	}
	return registry, nil
}

func (r ModuleRegistry) runnerFor(algorithm assessmentmodel.Algorithm) (algorithmRunner, error) {
	module, ok := r.modules[algorithm]
	if !ok {
		return algorithmRunner{}, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
	}
	return algorithmRunner{
		adapter:          module.Adapter,
		outcomeAssembler: module.outcomeAssembler,
		reportBuilder:    module.reportBuilder,
	}, nil
}

func (r ModuleRegistry) Algorithms() []assessmentmodel.Algorithm {
	out := make([]assessmentmodel.Algorithm, 0, len(r.modules))
	for algorithm := range r.modules {
		out = append(out, algorithm)
	}
	return out
}

func (r ModuleRegistry) Len() int {
	return len(r.modules)
}
