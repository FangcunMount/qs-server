package runtime

import (
	"fmt"

	behavioralratingEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/behavioral_rating"
	cognitiveEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
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
	path, err := evaldomain.ExecutionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return interpretationreporting.NewScaleScoreProjector(deps.ScoreRepo), nil
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return interpretationreporting.NewBehavioralRatingScoreProjector(deps.ScoreRepo), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return interpretationreporting.NewCognitiveScoreProjector(deps.ScoreRepo), nil
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
	path, err := evaldomain.ExecutionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return scaleEvaluation.NewExecutor(deps.ScaleScorer), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return typologyEvaluation.MaterializeTypologyEvaluator(desc, registry, session.typologyExecutor)
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return behavioralratingEvaluation.NewExecutor(deps.ScaleScorer), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return cognitiveEvaluation.NewExecutor(deps.ScaleScorer), nil
	default:
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (interpretationreporting.ReportBuilder, error) {
	path, err := evaldomain.ExecutionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return interpretationreporting.NewScaleReportBuilder(deps.ScaleReportBuilder), nil
	case modelcatalog.ExecutionPathTypologyDescriptor:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return typologyEvaluation.MaterializeTypologyReportBuilder(desc, registry, session.typologyReportBuilder)
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return interpretationreporting.NewBehavioralRatingReportBuilder(deps.ScaleReportBuilder), nil
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return interpretationreporting.NewCognitiveReportBuilder(deps.ScaleReportBuilder), nil
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
