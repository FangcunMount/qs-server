package evaluationinput

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathProvider exposes the materialization path for an input provider.
type ExecutionPathProvider interface {
	ExecutionPath() modelcatalog.ExecutionPath
}

// ExecutionPathForProvider resolves the execution path for a model input provider.
func ExecutionPathForProvider(provider ModelInputProvider) (modelcatalog.ExecutionPath, error) {
	if provider == nil {
		return "", fmt.Errorf("model input provider is nil")
	}
	if pathProvider, ok := provider.(ExecutionPathProvider); ok {
		return pathProvider.ExecutionPath(), nil
	}
	return evaluation.ExecutionPathForDescriptor(evaluation.ModelDescriptorFromIdentity(provider.ExecutionIdentity()))
}
