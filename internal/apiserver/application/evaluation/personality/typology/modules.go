package typology

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	bigfiveadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/bigfive"
	mbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/mbti"
	sbtiadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/sbti"
)

// DefaultModules returns the built-in personality typology modules for composition root wiring.
func DefaultModules() []Module {
	return []Module{
		MBTIModule(),
		SBTIModule(),
		BigFiveModule(),
	}
}

// AllModules returns all built-in typology modules.
func AllModules() []Module {
	return DefaultModules()
}

// MBTIModule wires the MBTI typology adapter and report builder.
func MBTIModule() Module {
	return Module{
		Algorithm:        assessmentmodel.AlgorithmMBTI,
		CategoryLabel:    "MBTI",
		Adapter:          mbtiadapter.Adapter{},
		outcomeAssembler: assembleMBTIOutcome,
		reportBuilder:    buildMBTIReport,
	}
}

// SBTIModule wires the SBTI typology adapter and report builder.
func SBTIModule() Module {
	return Module{
		Algorithm:        assessmentmodel.AlgorithmSBTI,
		CategoryLabel:    "SBTI",
		Adapter:          sbtiadapter.Adapter{},
		outcomeAssembler: assembleSBTIOutcome,
		reportBuilder:    buildSBTIReport,
	}
}

// BigFiveModule wires the Big Five typology adapter and report builder.
func BigFiveModule() Module {
	return Module{
		Algorithm:        assessmentmodel.AlgorithmBigFive,
		CategoryLabel:    "Big Five",
		Adapter:          bigfiveadapter.Adapter{},
		outcomeAssembler: assembleBigFiveOutcome,
		reportBuilder:    buildBigFiveReport,
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
	for _, module := range AllModules() {
		if module.Algorithm == algorithm && module.CategoryLabel != "" {
			return module.CategoryLabel
		}
	}
	return strings.ToUpper(string(algorithm))
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
