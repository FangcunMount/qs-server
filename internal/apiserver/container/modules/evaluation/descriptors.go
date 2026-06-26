package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// WiringDeps groups evaluator/report-builder wiring dependencies.
type WiringDeps struct {
	ScaleReportBuilder report.ReportBuilder
	ScaleScorer        portruleengine.ScaleFactorScorer
	TypologyRegistry   typologyEvaluation.ModuleRegistry
}

// MaterializeEvaluators builds evaluators from descriptors.
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
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

// MaterializeReportBuilders builds report builders from descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]evaluationResult.ReportBuilder, error) {
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

// AssertRegistryKeyParity verifies descriptor/evaluator/builder/provider key alignment.
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

func materializeEvaluator(desc evaldomain.ModelDescriptor, deps WiringDeps) (execute.Evaluator, error) {
	switch desc.Kind {
	case evaldomain.ModelKindScale:
		return scaleEvaluation.NewExecutor(deps.ScaleScorer), nil
	case evaldomain.ModelKindTypology:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return typologyEvaluation.NewTypologyExecutorWithRegistry(registry, desc.Algorithm)
	default:
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps WiringDeps) (evaluationResult.ReportBuilder, error) {
	switch desc.Kind {
	case evaldomain.ModelKindScale:
		return evaluationResult.NewScaleReportBuilder(deps.ScaleReportBuilder), nil
	case evaldomain.ModelKindTypology:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return typologyEvaluation.NewReportBuilderWithRegistry(registry, desc.Algorithm)
	default:
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}

func requireTypologyRegistry(deps WiringDeps) (typologyEvaluation.ModuleRegistry, error) {
	if deps.TypologyRegistry.Len() == 0 {
		return typologyEvaluation.ModuleRegistry{}, fmt.Errorf("typology registry is required")
	}
	return deps.TypologyRegistry, nil
}
