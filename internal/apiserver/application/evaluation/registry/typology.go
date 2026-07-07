package registry

import (
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type (
	// TypologyRegistry resolves typology runtime modules by algorithm alias.
	TypologyRegistry = factorclassification.ModuleRegistry
	// TypologyModule describes a built-in typology algorithm alias entry.
	TypologyModule = factorclassification.Module
	// TypologyRuntimeOptions injects typology outcome/report registries for wiring.
	TypologyRuntimeOptions = factorclassification.PersonalityRuntimeOptions
	// TypologyExecutor executes configured typology evaluations.
	TypologyExecutor = factorclassification.Executor
)

// DefaultTypologyModules returns built-in typology algorithm aliases.
func DefaultTypologyModules() []TypologyModule {
	return factorclassification.DefaultModules()
}

// DefaultTypologyRegistry builds the typology runtime registry for evaluation wiring.
func DefaultTypologyRegistry() (TypologyRegistry, error) {
	return factorclassification.DefaultModuleRegistry()
}

// TypologyRegistryWith builds a typology module registry with injectable runtime options.
func TypologyRegistryWith(opts TypologyRuntimeOptions) (TypologyRegistry, error) {
	return factorclassification.NewModuleRegistryWith(opts, DefaultTypologyModules()...)
}

// DefaultTypologyDescriptors projects the configured typology descriptor for evaluation wiring.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return factorclassification.DefaultTypologyDescriptors()
}

// NewConfiguredTypologyExecutor returns the default configured typology executor.
func NewConfiguredTypologyExecutor() (*TypologyExecutor, error) {
	return factorclassification.NewConfiguredTypologyExecutor()
}

// NewConfiguredReportBuilder returns the default configured typology report builder.
func NewConfiguredReportBuilder() (interpretationreporting.ReportBuilder, error) {
	return factorclassification.NewConfiguredReportBuilder()
}

// CategoryLabelFor resolves the display label for a typology algorithm.
func CategoryLabelFor(algorithm modelcatalog.Algorithm) string {
	return factorclassification.CategoryLabelFor(algorithm)
}
