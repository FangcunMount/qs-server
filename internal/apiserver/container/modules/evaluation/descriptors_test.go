package evaluation_test

import (
	"context"
	"testing"

	evalmodule "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAssertExecutionPathParityRejectsMismatchedProviderPath(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Kind: evaldomain.ModelKindScale},
	}
	providers := []evaluationinputInfra.ModelInputProvider{
		parityStubInputProvider{path: modelcatalog.ExecutionPathTypologyDescriptor},
	}

	err := evalmodule.AssertExecutionPathParity(descs, providers)
	if err == nil {
		t.Fatal("expected parity error for mismatched provider execution path")
	}
}

func TestAssertExecutionPathParityRejectsCountMismatch(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Kind: evaldomain.ModelKindScale},
	}
	err := evalmodule.AssertExecutionPathParity(descs, nil)
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
