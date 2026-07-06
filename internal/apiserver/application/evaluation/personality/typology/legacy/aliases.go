package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// DefaultAlgorithmAliases returns built-in typology algorithm aliases for legacy routing.
func DefaultAlgorithmAliases() []modelcatalog.Algorithm {
	return []modelcatalog.Algorithm{
		modelcatalog.AlgorithmMBTI,
		modelcatalog.AlgorithmSBTI,
		modelcatalog.AlgorithmBigFive,
	}
}

// CategoryLabelFor resolves the display label for a legacy typology algorithm alias.
func CategoryLabelFor(algorithm modelcatalog.Algorithm) string {
	switch algorithm {
	case modelcatalog.AlgorithmSBTI:
		return "SBTI"
	case modelcatalog.AlgorithmBigFive:
		return "Big Five"
	default:
		return "MBTI"
	}
}

// ReportSpecForAlgorithm derives report spec from a legacy algorithm identifier.
func ReportSpecForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.ReportSpec {
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm)
}

// OutcomeMappingForAlgorithm derives outcome mapping from a legacy algorithm identifier.
func OutcomeMappingForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.OutcomeMappingSpec {
	return modeltypology.LegacyOutcomeMappingFromAlgorithm(algorithm)
}
