package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

var defaultAlgorithmAliases = []modelcatalog.Algorithm{
	modelcatalog.AlgorithmMBTI,
	modelcatalog.AlgorithmSBTI,
	modelcatalog.AlgorithmBigFive,
}

// DefaultAlgorithmAliases returns built-in typology algorithm aliases for migration read paths.
//
// Deprecated: use MechanismReportBuilderKey / DecisionKind routing instead.
func DefaultAlgorithmAliases() []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, len(defaultAlgorithmAliases))
	copy(out, defaultAlgorithmAliases)
	return out
}

// CategoryLabelFor resolves display labels for legacy typology algorithm identifiers.
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

// ReportSpecForAlgorithm derives report spec from legacy algorithm identifiers.
func ReportSpecForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.ReportSpec {
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm)
}

// OutcomeMappingForAlgorithm derives outcome mapping from legacy algorithm identifiers.
func OutcomeMappingForAlgorithm(algorithm modelcatalog.Algorithm) modeltypology.OutcomeMappingSpec {
	return modeltypology.LegacyOutcomeMappingFromAlgorithm(algorithm)
}
