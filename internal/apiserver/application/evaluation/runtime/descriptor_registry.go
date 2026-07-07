package runtime

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DefaultRuntimeDescriptorRegistry registers mechanism descriptors aligned with materialize factories.
// Production wiring still uses ExecutionPath factories; this registry is the convergence bridge for Round 7.
func DefaultRuntimeDescriptorRegistry() (*evalpipeline.RuntimeDescriptorRegistry, error) {
	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	for _, path := range []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathTypologyDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	} {
		family, ok := algorithmFamilyForExecutionPath(path)
		if !ok {
			continue
		}
		if err := registry.Register(evalpipeline.RuntimeDescriptor{
			Key:             evalpipeline.RuntimeDescriptorKey{AlgorithmFamily: family},
			AlgorithmFamily: family,
			ExecutionPath:   path,
		}); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func algorithmFamilyForExecutionPath(path modelcatalog.ExecutionPath) (modelcatalog.AlgorithmFamily, bool) {
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return modelcatalog.AlgorithmFamilyFactorScoring, true
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return modelcatalog.AlgorithmFamilyFactorClassification, true
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return modelcatalog.AlgorithmFamilyFactorNorm, true
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return modelcatalog.AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}
