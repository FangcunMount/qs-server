package assembler

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type EvaluationWiringDeps struct {
	ScaleReportBuilder report.ReportBuilder
	ScaleScorer        portruleengine.ScaleFactorScorer
}

func DefaultEvaluationWiringDeps(scaleReportBuilder report.ReportBuilder) EvaluationWiringDeps {
	return EvaluationWiringDeps{
		ScaleReportBuilder: scaleReportBuilder,
		ScaleScorer:        ruleengine.NewScaleFactorScorer(),
	}
}

func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps EvaluationWiringDeps) ([]execute.Evaluator, error) {
	evaluators := make([]execute.Evaluator, 0, len(descs))
	for _, desc := range descs {
		evaluator, err := materializeEvaluator(desc, deps)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return evaluators, nil
}

func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps EvaluationWiringDeps) ([]evaluationResult.ReportBuilder, error) {
	builders := make([]evaluationResult.ReportBuilder, 0, len(descs))
	for _, desc := range descs {
		builder, err := materializeReportBuilder(desc, deps)
		if err != nil {
			return nil, err
		}
		builders = append(builders, builder)
	}
	return builders, nil
}

func AssertRegistryKeyParity(
	descs []evaldomain.ModelDescriptor,
	evaluators []execute.Evaluator,
	builders []evaluationResult.ReportBuilder,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	if len(descs) != len(evaluators) || len(descs) != len(builders) || len(descs) != len(providers) {
		return fmt.Errorf("evaluation descriptor count mismatch")
	}
	for i, desc := range descs {
		if evaluators[i].Key() != desc.Key {
			return fmt.Errorf("evaluator key mismatch at %d", i)
		}
		if builders[i].Key() != desc.Key {
			return fmt.Errorf("report builder key mismatch at %d", i)
		}
		if providers[i].EvaluatorKey() != desc.Key {
			return fmt.Errorf("input provider key mismatch at %d", i)
		}
	}
	return nil
}

func materializeEvaluator(desc evaldomain.ModelDescriptor, deps EvaluationWiringDeps) (execute.Evaluator, error) {
	switch desc.Kind {
	case evaldomain.ModelKindScale:
		return scaleEvaluation.NewExecutor(deps.ScaleScorer), nil
	case evaldomain.ModelKindTypology:
		return typologyEvaluation.NewTypologyExecutor(desc.Algorithm)
	default:
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps EvaluationWiringDeps) (evaluationResult.ReportBuilder, error) {
	switch desc.Kind {
	case evaldomain.ModelKindScale:
		return evaluationResult.NewScaleReportBuilder(deps.ScaleReportBuilder), nil
	case evaldomain.ModelKindTypology:
		return typologyEvaluation.NewReportBuilder(desc.Algorithm)
	default:
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}
