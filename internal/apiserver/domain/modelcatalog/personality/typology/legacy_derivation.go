package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"

// LegacyOutcomeMappingFromAlgorithm derives outcome mapping from a legacy algorithm identifier.
func LegacyOutcomeMappingFromAlgorithm(algorithm assessmentmodel.Algorithm) OutcomeMappingSpec {
	switch algorithm {
	case assessmentmodel.AlgorithmBigFive:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailTraitProfile,
			DetailAdapterKey: DetailAdapterBigFive,
			Algorithm:        algorithm,
		}
	case assessmentmodel.AlgorithmSBTI:
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
func LegacyReportSpecFromAlgorithm(algorithm assessmentmodel.Algorithm) ReportSpec {
	switch algorithm {
	case assessmentmodel.AlgorithmBigFive:
		return ReportSpec{
			Kind:          ReportKindTraitProfile,
			AdapterKey:    ReportAdapterBigFive,
			CategoryLabel: "Big Five",
		}
	case assessmentmodel.AlgorithmSBTI:
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
