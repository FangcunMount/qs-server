package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

// WiringDeps groups evaluator/report-builder wiring dependencies.
type WiringDeps = evalruntime.WiringDeps

// MaterializeEvaluators builds evaluators from descriptors.
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	return evalruntime.MaterializeEvaluators(descs, deps)
}

// MaterializeReportBuilders builds report builders from descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ReportBuilder, error) {
	return evalruntime.MaterializeReportBuilders(descs, deps)
}

// MaterializeScoreProjectors builds score projectors from descriptors.
func MaterializeScoreProjectors(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ScoreProjector, error) {
	return evalruntime.MaterializeScoreProjectors(descs, deps)
}

// AssertRegistryKeyParity verifies descriptor/evaluator/builder/provider key alignment.
// ModelDescriptor slices are the single source of truth for execute/input/report registries.
func AssertRegistryKeyParity(
	descs []evaldomain.ModelDescriptor,
	evaluators []execute.Evaluator,
	builders []interpretationreporting.ReportBuilder,
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
