package modelcatalog

import (
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// DefaultTypologyModules returns built-in typology algorithm aliases.
func DefaultTypologyModules() []evalregistry.TypologyModule {
	return evalregistry.DefaultTypologyModules()
}

// DefaultTypologyRegistry builds the typology runtime registry for evaluation wiring.
func DefaultTypologyRegistry() (evalregistry.TypologyRegistry, error) {
	return evalregistry.DefaultTypologyRegistry()
}

// TypologyRegistryWith builds a typology module registry with injectable adapter registries.
func TypologyRegistryWith(opts evalregistry.TypologyRuntimeOptions) (evalregistry.TypologyRegistry, error) {
	return evalregistry.TypologyRegistryWith(opts)
}

// DefaultTypologyDescriptors projects the configured typology descriptor for evaluation wiring.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return evalregistry.DefaultTypologyDescriptors()
}

// DefaultEvaluationDescriptors returns runtime descriptors for all capability-backed execution paths.
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	return evalregistry.DefaultEvaluationDescriptors()
}
