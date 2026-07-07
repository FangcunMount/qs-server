package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

// WiringDeps groups evaluator/report-builder wiring dependencies.
type WiringDeps = evalruntime.WiringDeps

// MaterializeEvaluators builds evaluators from descriptors.
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	return evalruntime.MaterializeEvaluators(descs, deps)
}

// MaterializeFamilyEvaluators builds one evaluator per algorithm family.
func MaterializeFamilyEvaluators(deps WiringDeps) (map[modelcatalog.AlgorithmFamily]execute.Evaluator, error) {
	return evalruntime.MaterializeFamilyEvaluators(deps)
}

// MaterializeLegacyEvaluators builds typology legacy alias evaluators.
func MaterializeLegacyEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	return evalruntime.MaterializeLegacyEvaluators(descs, deps)
}

// MaterializeReportBuilders builds report builders from descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ReportBuilder, error) {
	return evalruntime.MaterializeReportBuilders(descs, deps)
}

// MaterializeScoreProjectors builds score projectors from descriptors.
func MaterializeScoreProjectors(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]interpretationreporting.ScoreProjector, error) {
	return evalruntime.MaterializeScoreProjectors(descs, deps)
}

// AssertExecutionPathParity verifies descriptor/evaluator/builder/provider execution-path alignment.
// ModelDescriptor slices are the single source of truth for execute/input/report registries.
func AssertExecutionPathParity(
	descs []evaldomain.ModelDescriptor,
	evaluators []execute.Evaluator,
	builders []interpretationreporting.ReportBuilder,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	if len(descs) != len(evaluators) || len(descs) != len(builders) || len(descs) != len(providers) {
		return fmt.Errorf("evaluation descriptor count mismatch")
	}
	for i, desc := range descs {
		want, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			return fmt.Errorf("descriptor execution path at %d: %w", i, err)
		}
		evaluatorPath, err := execute.ExecutionPathForEvaluator(evaluators[i])
		if err != nil {
			return fmt.Errorf("evaluator execution path at %d: %w", i, err)
		}
		if evaluatorPath != want {
			return fmt.Errorf("evaluator execution path mismatch at %d: got %s want %s", i, evaluatorPath, want)
		}
		builderPath, err := interpretationreporting.ExecutionPathForReportBuilder(builders[i])
		if err != nil {
			return fmt.Errorf("report builder execution path at %d: %w", i, err)
		}
		if builderPath != want {
			return fmt.Errorf("report builder execution path mismatch at %d: got %s want %s", i, builderPath, want)
		}
		providerPath, err := evaluationinputInfra.ExecutionPathForProvider(providers[i])
		if err != nil {
			return fmt.Errorf("input provider execution path at %d: %w", i, err)
		}
		if providerPath != want {
			return fmt.Errorf("input provider execution path mismatch at %d: got %s want %s", i, providerPath, want)
		}
	}
	return nil
}

// AssertRegistryKeyParity is deprecated; use AssertExecutionPathParity.
func AssertRegistryKeyParity(
	descs []evaldomain.ModelDescriptor,
	evaluators []execute.Evaluator,
	builders []interpretationreporting.ReportBuilder,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	return AssertExecutionPathParity(descs, evaluators, builders, providers)
}
