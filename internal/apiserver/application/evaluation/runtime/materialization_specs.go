package runtime

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type pathMaterialization struct {
	path   modelcatalog.ExecutionPath
	family modelcatalog.AlgorithmFamily
}

func defaultPathMaterializations() []pathMaterialization {
	return []pathMaterialization{
		{path: modelcatalog.ExecutionPathScaleDescriptor, family: modelcatalog.AlgorithmFamilyFactorScoring},
		{path: modelcatalog.ExecutionPathTypologyDescriptor, family: modelcatalog.AlgorithmFamilyFactorClassification},
		{path: modelcatalog.ExecutionPathBehavioralRatingDescriptor, family: modelcatalog.AlgorithmFamilyFactorNorm},
		{path: modelcatalog.ExecutionPathCognitiveDescriptor, family: modelcatalog.AlgorithmFamilyTaskPerformance},
	}
}

func materializationOrder() []modelcatalog.ExecutionPath {
	specs := defaultPathMaterializations()
	paths := make([]modelcatalog.ExecutionPath, len(specs))
	for i, spec := range specs {
		paths[i] = spec.path
	}
	return paths
}

func runtimeDescriptorsFromSpecs(specs []pathMaterialization) ([]evalpipeline.RuntimeDescriptor, error) {
	descs := make([]evalpipeline.RuntimeDescriptor, 0, len(specs))
	for _, spec := range specs {
		decisionKind := evalrouting.DecisionKindForFamily(spec.family)
		descs = append(descs, evalpipeline.RuntimeDescriptor{
			Key: evalpipeline.DescriptorKey{
				AlgorithmFamily: spec.family,
			},
			AlgorithmFamily: spec.family,
			DecisionKind:    decisionKind,
			ExecutionPath:   spec.path,
		})
	}
	return descs, nil
}

func algorithmFamilyForExecutionPath(path modelcatalog.ExecutionPath) (modelcatalog.AlgorithmFamily, bool) {
	for _, spec := range defaultPathMaterializations() {
		if spec.path == path {
			return spec.family, true
		}
	}
	return "", false
}
