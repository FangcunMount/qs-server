package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
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

// AssertExecutionPathParity verifies descriptor/evaluator/provider execution-path alignment.
// ModelDescriptor slices are the single source of truth for Evaluation execute/input registries.
func AssertExecutionPathParity(
	descs []evaldomain.ModelDescriptor,
	evaluators []execute.Evaluator,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	if len(descs) != len(evaluators) || len(descs) != len(providers) {
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
