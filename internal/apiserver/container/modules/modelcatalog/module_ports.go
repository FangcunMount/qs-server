package modelcatalog

import (
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
)

// ExportEvaluationCatalog builds descriptors and typology registry owned by assessmentmodel.
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

// ExportEvaluationCatalog exposes the default evaluation catalog from the aggregate module.
func (m *Module) ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	if m == nil {
		return compose.EvaluationCatalog{}, nil
	}
	return ExportEvaluationCatalog()
}
