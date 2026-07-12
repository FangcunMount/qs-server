package modelcatalog

import (
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
)

// ExportEvaluationCatalog exposes the single Evaluation runtime registry.
func ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	runtimeRegistry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		return compose.EvaluationCatalog{}, err
	}
	return compose.EvaluationCatalog{
		RuntimeDescriptorRegistry: runtimeRegistry,
	}, nil
}

// ExportEvaluationCatalog 暴露模型目录的默认评估目录
func (m *Module) ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	if m == nil {
		return compose.EvaluationCatalog{}, nil
	}
	return ExportEvaluationCatalog()
}
