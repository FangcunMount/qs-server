package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_norm"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/task_performance"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type evaluatorFactory func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error)

type reportBuilderFactory func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error)

type scoreProjectorFactory func(deps WiringDeps) (interpretationreporting.ScoreProjector, error)

var evaluatorFactories = map[modelcatalog.ExecutionPath]evaluatorFactory{
	modelcatalog.ExecutionPathScaleDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
		return factorscoring.NewExecutor(deps.ScaleScorer), nil
	},
	modelcatalog.ExecutionPathTypologyDescriptor: func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error) {
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return factorclassification.MaterializeEvaluator(desc, registry, session.typologyExecutor)
	},
	modelcatalog.ExecutionPathBehavioralRatingDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
		return factornorm.NewExecutor(deps.ScaleScorer), nil
	},
	modelcatalog.ExecutionPathCognitiveDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (execute.Evaluator, error) {
		return taskperformance.NewExecutor(deps.ScaleScorer), nil
	},
}

var reportBuilderFactories = map[modelcatalog.ExecutionPath]reportBuilderFactory{
	modelcatalog.ExecutionPathScaleDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewFactorScoringReportBuilder(deps.ScaleReportBuilder), nil
	},
	modelcatalog.ExecutionPathTypologyDescriptor: func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error) {
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return factorclassification.MaterializeReportBuilder(desc, registry, session.typologyReportBuilder)
	},
	modelcatalog.ExecutionPathBehavioralRatingDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewNormProfileReportBuilder(deps.ScaleReportBuilder), nil
	},
	modelcatalog.ExecutionPathCognitiveDescriptor: func(_ evaldomain.ModelDescriptor, deps WiringDeps, _ wiringSession) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewTaskPerformanceReportBuilder(deps.ScaleReportBuilder), nil
	},
}

var scoreProjectorFactories = map[modelcatalog.ExecutionPath]scoreProjectorFactory{
	modelcatalog.ExecutionPathScaleDescriptor: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
		return interpretationreporting.NewFactorScoringScoreProjector(deps.ScoreRepo), nil
	},
	modelcatalog.ExecutionPathBehavioralRatingDescriptor: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
		return interpretationreporting.NewNormProfileScoreProjector(deps.ScoreRepo), nil
	},
	modelcatalog.ExecutionPathCognitiveDescriptor: func(deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
		return interpretationreporting.NewTaskPerformanceScoreProjector(deps.ScoreRepo), nil
	},
}

// WiringDeps groups shared runtime materialization dependencies.
type WiringDeps struct {
	ScaleReportBuilder report.ReportBuilder
	ScaleScorer        portruleengine.ScaleFactorScorer
	ScoreRepo          assessment.ScoreRepository
	TypologyRegistry   factorclassification.ModuleRegistry
}

type wiringSession struct {
	typologyExecutor      **factorclassification.Executor
	typologyReportBuilder *factorclassification.ReportBuilder
}

// MaterializeEvaluators builds evaluators from descriptors.
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	var sharedConfigured *factorclassification.Executor
	session := wiringSession{typologyExecutor: &sharedConfigured}
	evaluators := make([]execute.Evaluator, 0, len(descs))
	for _, desc := range descs {
		evaluator, err := materializeEvaluator(desc, deps, session)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return evaluators, nil
}

// MaterializeScoreProjectors builds score projectors for descriptor-backed scale-like runtimes.
func MaterializeScoreProjectors(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ScoreProjector, error) {
	if deps.ScoreRepo == nil {
		return nil, fmt.Errorf("score repository is required")
	}
	projectors := make([]interpretationreporting.ScoreProjector, 0, len(descs))
	for _, desc := range descs {
		projector, err := materializeScoreProjector(desc, deps)
		if err != nil {
			return nil, err
		}
		if projector != nil {
			projectors = append(projectors, projector)
		}
	}
	return projectors, nil
}

func materializeScoreProjector(desc evaldomain.ModelDescriptor, deps WiringDeps) (interpretationreporting.ScoreProjector, error) {
	path, err := executionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	factory, ok := scoreProjectorFactories[path]
	if !ok {
		if path == modelcatalog.ExecutionPathTypologyDescriptor {
			return nil, nil
		}
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
	return factory(deps)
}

// MaterializeReportBuilders builds report builders from descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ReportBuilder, error) {
	var sharedConfigured factorclassification.ReportBuilder
	session := wiringSession{typologyReportBuilder: &sharedConfigured}
	builders := make([]interpretationreporting.ReportBuilder, 0, len(descs))
	for _, desc := range descs {
		builder, err := materializeReportBuilder(desc, deps, session)
		if err != nil {
			return nil, err
		}
		builders = append(builders, builder)
	}
	return builders, nil
}

func materializeEvaluator(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error) {
	path, err := executionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	factory, ok := evaluatorFactories[path]
	if !ok {
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
	return factory(desc, deps, session)
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error) {
	path, err := executionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	factory, ok := reportBuilderFactories[path]
	if !ok {
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
	return factory(desc, deps, session)
}

func requireTypologyRegistry(deps WiringDeps) (factorclassification.ModuleRegistry, error) {
	if deps.TypologyRegistry.Len() == 0 {
		return factorclassification.ModuleRegistry{}, fmt.Errorf("typology registry is required")
	}
	return deps.TypologyRegistry, nil
}

func executionPathForDescriptor(desc evaldomain.ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(desc.Kind))
}
