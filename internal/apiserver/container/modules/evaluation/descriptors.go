package evaluation

import (
	"fmt"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

// WiringDeps groups native descriptor-pipeline wiring dependencies.
type WiringDeps = evalruntime.WiringDeps

// AssertExecutionPathParity verifies runtime-path/provider alignment.
func AssertExecutionPathParity(
	paths []modelcatalog.ExecutionPath,
	providers []evaluationinputInfra.ModelInputProvider,
) error {
	if len(paths) != len(providers) {
		return fmt.Errorf("evaluation execution path count mismatch")
	}
	for i, want := range paths {
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
