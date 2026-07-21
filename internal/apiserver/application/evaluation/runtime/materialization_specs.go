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
	manifest := RequiredFamilyManifest()
	specs := make([]pathMaterialization, 0, len(manifest))
	for _, entry := range manifest {
		specs = append(specs, pathMaterialization{path: entry.Path, family: entry.Family})
	}
	return specs
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
		for _, decisionKind := range decisionKindsForFamily(spec.family) {
			descs = append(descs, evalpipeline.RuntimeDescriptor{
				Key:             evalpipeline.DescriptorKey{AlgorithmFamily: spec.family, DecisionKind: decisionKind},
				AlgorithmFamily: spec.family,
				DecisionKind:    decisionKind,
				ExecutionPath:   spec.path,
			})
		}
	}
	return descs, nil
}

func decisionKindsForFamily(family modelcatalog.AlgorithmFamily) []modelcatalog.DecisionKind {
	if family == modelcatalog.AlgorithmFamilyFactorClassification {
		return []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindPoleComposition,
			modelcatalog.DecisionKindTraitProfile,
			modelcatalog.DecisionKindNearestPattern,
			modelcatalog.DecisionKindDominantFactor,
		}
	}
	if decision := evalrouting.DecisionKindForFamily(family); decision != "" {
		return []modelcatalog.DecisionKind{decision}
	}
	return nil
}

func algorithmFamilyForExecutionPath(path modelcatalog.ExecutionPath) (modelcatalog.AlgorithmFamily, bool) {
	for _, spec := range defaultPathMaterializations() {
		if spec.path == path {
			return spec.family, true
		}
	}
	return "", false
}
