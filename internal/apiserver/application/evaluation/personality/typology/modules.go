package typology

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	mbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/mbti"
	sbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/sbti"
)

// DefaultModules returns the built-in personality typology modules for composition root wiring.
func DefaultModules() []Module {
	return []Module{
		MBTIModule(),
		SBTIModule(),
	}
}

// MBTIModule wires the MBTI typology adapter and report builder.
func MBTIModule() Module {
	return Module{
		Algorithm:     assessmentmodel.AlgorithmMBTI,
		CategoryLabel: "MBTI",
		Adapter:       mbtiadapter.Adapter{},
		reportBuilder: buildMBTIReport,
	}
}

// SBTIModule wires the SBTI typology adapter and report builder.
func SBTIModule() Module {
	return Module{
		Algorithm:     assessmentmodel.AlgorithmSBTI,
		CategoryLabel: "SBTI",
		Adapter:       sbtiadapter.Adapter{},
		reportBuilder: buildSBTIReport,
	}
}

// DefaultAlgorithms returns algorithms registered by DefaultModules.
func DefaultAlgorithms() []assessmentmodel.Algorithm {
	modules := DefaultModules()
	out := make([]assessmentmodel.Algorithm, 0, len(modules))
	for _, module := range modules {
		out = append(out, module.Algorithm)
	}
	return out
}

// CategoryLabelFor resolves the display label for a typology algorithm.
func CategoryLabelFor(algorithm assessmentmodel.Algorithm) string {
	for _, module := range DefaultModules() {
		if module.Algorithm == algorithm && module.CategoryLabel != "" {
			return module.CategoryLabel
		}
	}
	switch algorithm {
	case assessmentmodel.AlgorithmBigFive:
		return "Big Five"
	default:
		return strings.ToUpper(string(algorithm))
	}
}

// DefaultModuleRegistry builds the default typology module registry.
func DefaultModuleRegistry() (ModuleRegistry, error) {
	return NewModuleRegistry(DefaultModules()...)
}

func mustDefaultModuleRegistry() ModuleRegistry {
	registry, err := DefaultModuleRegistry()
	if err != nil {
		panic(fmt.Sprintf("default typology module registry: %v", err))
	}
	return registry
}
