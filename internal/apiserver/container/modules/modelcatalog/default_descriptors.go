package modelcatalog

import (
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DefaultTypologyModules returns built-in typology algorithm aliases.
func DefaultTypologyModules() []typologyEvaluation.Module {
	return typologyEvaluation.DefaultModules()
}

// DefaultTypologyRegistry builds the typology runtime registry for evaluation wiring.
func DefaultTypologyRegistry() (typologyEvaluation.ModuleRegistry, error) {
	return typologyEvaluation.DefaultPersonalityRuntimeRegistry().AsModuleRegistry(), nil
}

// TypologyRegistryWith builds a typology module registry with injectable adapter registries.
func TypologyRegistryWith(opts typologyEvaluation.PersonalityRuntimeOptions) (typologyEvaluation.ModuleRegistry, error) {
	return typologyEvaluation.NewPersonalityRuntimeRegistryWith(opts).AsModuleRegistry(), nil
}

// DefaultTypologyDescriptors projects the configured typology descriptor for evaluation wiring.
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return typologyEvaluation.DefaultTypologyDescriptors()
}

var defaultRuntimeExecutionPathOrder = []domain.ExecutionPath{
	domain.ExecutionPathScaleDescriptor,
	domain.ExecutionPathTypologyDescriptor,
	domain.ExecutionPathBehavioralRatingDescriptor,
	domain.ExecutionPathCognitiveDescriptor,
}

// DefaultEvaluationDescriptors returns runtime descriptors for all capability-backed execution paths.
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	runtimePaths := make(map[domain.ExecutionPath]bool)
	for _, cap := range domain.ModelFamilyCapabilitiesV2() {
		if cap.RuntimeExecutable {
			runtimePaths[cap.ExecutionPath] = true
		}
	}

	var descs []evaldomain.ModelDescriptor
	for _, path := range defaultRuntimeExecutionPathOrder {
		if !runtimePaths[path] {
			continue
		}
		descs = append(descs, descriptorsForExecutionPath(path)...)
	}
	return descs
}

func descriptorsForExecutionPath(path domain.ExecutionPath) []evaldomain.ModelDescriptor {
	switch path {
	case domain.ExecutionPathScaleDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.ScaleModelDescriptor()}
	case domain.ExecutionPathTypologyDescriptor:
		return DefaultTypologyDescriptors()
	case domain.ExecutionPathBehavioralRatingDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.BehavioralRatingModelDescriptor()}
	case domain.ExecutionPathCognitiveDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.CognitiveModelDescriptor()}
	default:
		return nil
	}
}
