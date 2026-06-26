package assessmentmodel

import (
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// DefaultTypologyModules returns built-in typology algorithm aliases.
func DefaultTypologyModules() []typologyEvaluation.Module {
	return typologyEvaluation.DefaultModules()
}

// DefaultTypologyRegistry builds the typology runtime registry for evaluation wiring.
func DefaultTypologyRegistry() (typologyEvaluation.ModuleRegistry, error) {
	return typologyEvaluation.DefaultPersonalityRuntimeRegistry().AsModuleRegistry(), nil
}

// DefaultTypologyDescriptors projects the configured typology descriptor for evaluation wiring.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return typologyEvaluation.DefaultTypologyDescriptors()
}

// DefaultEvaluationDescriptors returns scale + built-in typology descriptors for evaluation/input wiring.
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	scale := evaldomain.ModelDescriptor{
		Key:  evaldomain.EvaluatorKeyScaleDefault,
		Kind: evaldomain.ModelKindScale,
	}
	return append([]evaldomain.ModelDescriptor{scale}, DefaultTypologyDescriptors()...)
}
