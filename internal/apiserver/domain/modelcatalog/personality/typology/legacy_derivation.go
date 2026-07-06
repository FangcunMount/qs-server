package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// LegacyOutcomeMappingFromAlgorithm derives outcome mapping from a legacy algorithm identifier.
func LegacyOutcomeMappingFromAlgorithm(algorithm modelcatalog.Algorithm) OutcomeMappingSpec {
	switch algorithm {
	case modelcatalog.AlgorithmBigFive:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailTraitProfile,
			DetailAdapterKey: DetailAdapterBigFive,
			Algorithm:        algorithm,
		}
	case modelcatalog.AlgorithmSBTI:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailPersonalityType,
			DetailAdapterKey: DetailAdapterSBTI,
			Algorithm:        algorithm,
		}
	default:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailPersonalityType,
			DetailAdapterKey: DetailAdapterMBTI,
			Algorithm:        algorithm,
		}
	}
}

// LegacyReportSpecFromPayload derives report spec from a legacy payload algorithm field.
func LegacyReportSpecFromPayload(p *Payload) ReportSpec {
	if p == nil {
		return ReportSpec{}
	}
	return LegacyReportSpecFromAlgorithm(p.Algorithm)
}

// LegacyReportSpecFromAlgorithm derives report spec from a legacy algorithm identifier.
func LegacyReportSpecFromAlgorithm(algorithm modelcatalog.Algorithm) ReportSpec {
	switch algorithm {
	case modelcatalog.AlgorithmBigFive:
		return ReportSpec{
			Kind:          ReportKindTraitProfile,
			AdapterKey:    ReportAdapterBigFive,
			CategoryLabel: "Big Five",
		}
	case modelcatalog.AlgorithmSBTI:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterSBTI,
			CategoryLabel: "SBTI",
		}
	default:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterMBTI,
			CategoryLabel: "MBTI",
		}
	}
}
