package evaluation_test

import (
	"context"
	"fmt"
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// assertExecutionPathParity verifies runtime-path/provider alignment.
func assertExecutionPathParity(
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

func TestAssertExecutionPathParityRejectsMismatchedProviderPath(t *testing.T) {
	paths := []modelcatalog.ExecutionPath{modelcatalog.ExecutionPathScaleDescriptor}
	providers := []evaluationinputInfra.ModelInputProvider{
		parityStubInputProvider{path: modelcatalog.ExecutionPathTypologyDescriptor},
	}

	err := assertExecutionPathParity(paths, providers)
	if err == nil {
		t.Fatal("expected parity error for mismatched provider execution path")
	}
}

func TestAssertExecutionPathParityRejectsCountMismatch(t *testing.T) {
	paths := []modelcatalog.ExecutionPath{modelcatalog.ExecutionPathScaleDescriptor}
	err := assertExecutionPathParity(paths, nil)
	if err == nil {
		t.Fatal("expected parity error for descriptor count mismatch")
	}
}

type parityStubInputProvider struct {
	path modelcatalog.ExecutionPath
}

func (s parityStubInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityScaleDefault
}

func (s parityStubInputProvider) ExecutionPath() modelcatalog.ExecutionPath { return s.path }

func (parityStubInputProvider) ResolveInput(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, nil
}
