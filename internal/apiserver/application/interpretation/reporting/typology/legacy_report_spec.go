package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// ReportBuildContextFromAlgorithm resolves legacy report context when published payload has no runtime spec.
func reportBuildContextFromAlgorithm(algorithm modelcatalog.Algorithm) (modeltypology.ReportSpec, modeltypology.OutcomeMappingSpec, modelcatalog.DecisionKind) {
	if algorithm == "" {
		return modeltypology.ReportSpec{}, modeltypology.OutcomeMappingSpec{}, ""
	}
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm),
		modeltypology.LegacyOutcomeMappingFromAlgorithm(algorithm),
		""
}
