package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_norm"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_scoring"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/task_performance"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// WiringDeps groups shared runtime materialization dependencies.
type WiringDeps struct {
	ScaleReportBuilder report.ReportBuilder
	ScaleScorer        portruleengine.ScaleFactorScorer
	ScoreRepo          assessment.ScoreRepository
	TypologyRegistry   typologyEvaluation.ModuleRegistry
}

type wiringSession struct {
	typologyExecutor      **typologyEvaluation.Executor
	typologyReportBuilder *typologyEvaluation.ReportBuilder
}

// MaterializeEvaluators builds evaluators from descriptors.
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	var sharedConfigured *typologyEvaluation.Executor
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
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return interpretationreporting.NewFactorScoringScoreProjector(deps.ScoreRepo), nil
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return interpretationreporting.NewNormProfileScoreProjector(deps.ScoreRepo), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return interpretationreporting.NewTaskPerformanceScoreProjector(deps.ScoreRepo), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}

// MaterializeReportBuilders builds report builders from descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ReportBuilder, error) {
	var sharedConfigured typologyEvaluation.ReportBuilder
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
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return factorscoring.NewExecutor(deps.ScaleScorer), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return factorclassification.MaterializeEvaluator(desc, registry, session.typologyExecutor)
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return factornorm.NewExecutor(deps.ScaleScorer), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return taskperformance.NewExecutor(deps.ScaleScorer), nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error) {
	path, err := executionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return interpretationreporting.NewFactorScoringReportBuilder(deps.ScaleReportBuilder), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return factorclassification.MaterializeReportBuilder(desc, registry, session.typologyReportBuilder)
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return interpretationreporting.NewNormProfileReportBuilder(deps.ScaleReportBuilder), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return interpretationreporting.NewTaskPerformanceReportBuilder(deps.ScaleReportBuilder), nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}

func requireTypologyRegistry(deps WiringDeps) (typologyEvaluation.ModuleRegistry, error) {
	if deps.TypologyRegistry.Len() == 0 {
		return typologyEvaluation.ModuleRegistry{}, fmt.Errorf("typology registry is required")
	}
	return deps.TypologyRegistry, nil
}

func executionPathForDescriptor(desc evaldomain.ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(desc.Kind))
}
