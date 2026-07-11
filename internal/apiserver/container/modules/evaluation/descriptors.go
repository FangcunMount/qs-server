package evaluation

import (
	"fmt"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

// WiringDeps groups native descriptor-pipeline wiring dependencies.
type WiringDeps = evalruntime.WiringDeps

// AssertExecutionPathParity verifies descriptor/provider execution-path alignment.
// ModelDescriptor slices are the single source of truth for Evaluation input registration.
func AssertExecutionPathParity(
	descs []evaldomain.ModelDescriptor,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	if len(descs) != len(providers) {
		return fmt.Errorf("evaluation descriptor count mismatch")
	}
	for i, desc := range descs {
		want, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			return fmt.Errorf("descriptor execution path at %d: %w", i, err)
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
