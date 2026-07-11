package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type pathMaterialization struct {
	path      modelcatalog.ExecutionPath
	family    modelcatalog.AlgorithmFamily
	evaluator evaluatorFactory
}

func defaultPathMaterializations() []pathMaterialization {
	return []pathMaterialization{
		{
			path:   modelcatalog.ExecutionPathScaleDescriptor,
			family: modelcatalog.AlgorithmFamilyFactorScoring,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return factorscoring.NewExecutor(deps.ScaleScorer), nil
			},
		},
		{
			path:   modelcatalog.ExecutionPathTypologyDescriptor,
			family: modelcatalog.AlgorithmFamilyFactorClassification,
			evaluator: func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error) {
				registry, err := requireTypologyRegistry(deps)
				if err != nil {
					return nil, err
				}
				return factorclassification.MaterializeEvaluator(desc, registry, session.typologyExecutor)
			},
		},
		{
			path:   modelcatalog.ExecutionPathBehavioralRatingDescriptor,
			family: modelcatalog.AlgorithmFamilyFactorNorm,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return factornorm.NewExecutor(deps.ScaleScorer), nil
			},
		},
		{
			path:   modelcatalog.ExecutionPathCognitiveDescriptor,
			family: modelcatalog.AlgorithmFamilyTaskPerformance,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return taskperformance.NewExecutor(deps.ScaleScorer), nil
			},
		},
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

func buildFactoryMaps(specs []pathMaterialization) (
	map[modelcatalog.ExecutionPath]evaluatorFactory,
	error,
) {
	evaluators := make(map[modelcatalog.ExecutionPath]evaluatorFactory, len(specs))
	for _, spec := range specs {
		if spec.evaluator == nil {
			return nil, fmt.Errorf("materialization spec %s missing evaluator factory", spec.path)
		}
		evaluators[spec.path] = spec.evaluator
	}
	return evaluators, nil
}

func runtimeDescriptorsFromSpecs(specs []pathMaterialization) ([]evalpipeline.RuntimeDescriptor, error) {
	descs := make([]evalpipeline.RuntimeDescriptor, 0, len(specs))
	for _, spec := range specs {
		decisionKind := defaultDecisionKindForFamily(spec.family)
		descs = append(descs, evalpipeline.RuntimeDescriptor{
			Key: evalpipeline.RuntimeDescriptorKey{
				AlgorithmFamily: spec.family,
			},
			AlgorithmFamily: spec.family,
			DecisionKind:    decisionKind,
			ExecutionPath:   spec.path,
		})
	}
	return descs, nil
}

func defaultDecisionKindForFamily(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
}

func algorithmFamilyForExecutionPath(path modelcatalog.ExecutionPath) (modelcatalog.AlgorithmFamily, bool) {
	for _, spec := range defaultPathMaterializations() {
		if spec.path == path {
			return spec.family, true
		}
	}
	return "", false
}
