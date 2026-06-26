package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

// DefaultAlgorithmAliases returns built-in typology algorithm aliases for legacy routing.
func DefaultAlgorithmAliases() []assessmentmodel.Algorithm {
	return []assessmentmodel.Algorithm{
		assessmentmodel.AlgorithmMBTI,
		assessmentmodel.AlgorithmSBTI,
		assessmentmodel.AlgorithmBigFive,
	}
}

// CategoryLabelFor resolves the display label for a legacy typology algorithm alias.
func CategoryLabelFor(algorithm assessmentmodel.Algorithm) string {
	switch algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return "SBTI"
	case assessmentmodel.AlgorithmBigFive:
		return "Big Five"
	default:
		return "MBTI"
	}
}

// ReportSpecForAlgorithm derives report spec from a legacy algorithm identifier.
func ReportSpecForAlgorithm(algorithm assessmentmodel.Algorithm) modeltypology.ReportSpec {
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm)
}

// OutcomeMappingForAlgorithm derives outcome mapping from a legacy algorithm identifier.
func OutcomeMappingForAlgorithm(algorithm assessmentmodel.Algorithm) modeltypology.OutcomeMappingSpec {
	return modeltypology.LegacyOutcomeMappingFromAlgorithm(algorithm)
}
