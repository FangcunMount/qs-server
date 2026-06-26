package assessmentmodel

import (
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// DefaultTypologyModules returns built-in typology modules owned by assessment-model composition.
func DefaultTypologyModules() []typologyEvaluation.Module {
	return typologyEvaluation.DefaultModules()
}

// DefaultTypologyRegistry builds the typology module registry for evaluation wiring.
func DefaultTypologyRegistry() (typologyEvaluation.ModuleRegistry, error) {
	return typologyEvaluation.NewModuleRegistry(DefaultTypologyModules()...)
}

// DefaultTypologyDescriptors projects built-in typology modules to evaluation descriptors.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return typologyEvaluation.ModuleDescriptors(DefaultTypologyModules())
}

// DefaultEvaluationDescriptors returns scale + built-in typology descriptors for evaluation/input wiring.
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	scale := evaldomain.ModelDescriptor{
		Key:  evaldomain.EvaluatorKeyScaleDefault,
		Kind: evaldomain.ModelKindScale,
	}
	return append([]evaldomain.ModelDescriptor{scale}, DefaultTypologyDescriptors()...)
}
