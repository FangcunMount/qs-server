package assessmentmodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
)

// ExportEvaluationCatalog builds descriptors and typology registry owned by assessmentmodel.
func ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	registry, err := DefaultTypologyRegistry()
	if err != nil {
		return compose.EvaluationCatalog{}, err
	}
	return compose.EvaluationCatalog{
		Descriptors:      DefaultEvaluationDescriptors(),
		TypologyRegistry: registry,
	}, nil
}

// ExportEvaluationCatalog exposes the default evaluation catalog from the aggregate module.
func (m *Module) ExportEvaluationCatalog() (compose.EvaluationCatalog, error) {
	if m == nil {
		return compose.EvaluationCatalog{}, nil
	}
	return ExportEvaluationCatalog()
}
