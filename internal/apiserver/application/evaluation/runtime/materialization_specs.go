package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_classification"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_norm"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type pathMaterialization struct {
	path           modelcatalog.ExecutionPath
	family         modelcatalog.AlgorithmFamily
	evaluator      evaluatorFactory
	reportBuilder  reportBuilderFactory
	scoreProjector scoreProjectorFactory
}

func defaultPathMaterializations() []pathMaterialization {
	return []pathMaterialization{
		{
			path:   modelcatalog.ExecutionPathScaleDescriptor,
			family: modelcatalog.AlgorithmFamilyFactorScoring,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return factorscoring.NewExecutor(deps.ScaleScorer), nil
			},
			reportBuilder: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
				return interpretationreporting.NewFactorScoringReportBuilder(deps.ScaleReportBuilder), nil
			},
			scoreProjector: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
				return interpretationreporting.NewFactorScoringScoreProjector(deps.ScoreRepo), nil
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
			reportBuilder: func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error) {
				registry, err := requireTypologyRegistry(deps)
				if err != nil {
					return nil, err
				}
				return factorclassification.MaterializeReportBuilder(desc, registry, session.typologyReportBuilder)
			},
		},
		{
			path:   modelcatalog.ExecutionPathBehavioralRatingDescriptor,
			family: modelcatalog.AlgorithmFamilyFactorNorm,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return factornorm.NewExecutor(deps.ScaleScorer), nil
			},
			reportBuilder: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
				return interpretationreporting.NewNormProfileReportBuilder(deps.ScaleReportBuilder), nil
			},
			scoreProjector: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
				return interpretationreporting.NewNormProfileScoreProjector(deps.ScoreRepo), nil
			},
		},
		{
			path:   modelcatalog.ExecutionPathCognitiveDescriptor,
			family: modelcatalog.AlgorithmFamilyTaskPerformance,
			evaluator: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
				return taskperformance.NewExecutor(deps.ScaleScorer), nil
			},
			reportBuilder: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
				return interpretationreporting.NewTaskPerformanceReportBuilder(deps.ScaleReportBuilder), nil
			},
			scoreProjector: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
				return interpretationreporting.NewTaskPerformanceScoreProjector(deps.ScoreRepo), nil
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
	map[modelcatalog.ExecutionPath]reportBuilderFactory,
	map[modelcatalog.ExecutionPath]scoreProjectorFactory,
	error,
) {
	evaluators := make(map[modelcatalog.ExecutionPath]evaluatorFactory, len(specs))
	reports := make(map[modelcatalog.ExecutionPath]reportBuilderFactory, len(specs))
	projectors := make(map[modelcatalog.ExecutionPath]scoreProjectorFactory, len(specs))
	for _, spec := range specs {
		if spec.evaluator == nil {
			return nil, nil, nil, fmt.Errorf("materialization spec %s missing evaluator factory", spec.path)
		}
		if spec.reportBuilder == nil {
			return nil, nil, nil, fmt.Errorf("materialization spec %s missing report builder factory", spec.path)
		}
		evaluators[spec.path] = spec.evaluator
		reports[spec.path] = spec.reportBuilder
		if spec.scoreProjector != nil {
			projectors[spec.path] = spec.scoreProjector
		}
	}
	return evaluators, reports, projectors, nil
}

func runtimeDescriptorsFromSpecs(specs []pathMaterialization) ([]evalpipeline.RuntimeDescriptor, error) {
	descs := make([]evalpipeline.RuntimeDescriptor, 0, len(specs))
	for _, spec := range specs {
		descs = append(descs, evalpipeline.RuntimeDescriptor{
			Key:             evalpipeline.RuntimeDescriptorKey{AlgorithmFamily: spec.family},
			AlgorithmFamily: spec.family,
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
