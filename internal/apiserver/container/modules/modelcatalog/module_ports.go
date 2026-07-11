package modelcatalog

import (
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
)

// ExportEvaluationCatalog 构建模型目录的描述符和模型算法注册表
func ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	typologyRegistry, err := evalregistry.DefaultTypologyRegistry()
	if err != nil {
		return compose.EvaluationCatalog{}, err
	}
	runtimeRegistry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		return compose.EvaluationCatalog{}, err
	}
	return compose.EvaluationCatalog{
		Descriptors:               evalregistry.DefaultEvaluationDescriptors(),
		TypologyRegistry:          typologyRegistry,
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
