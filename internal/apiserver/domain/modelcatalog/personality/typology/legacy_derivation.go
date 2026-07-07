package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// LegacyOutcomeMappingFromAlgorithm 推导结果 mapping 从 旧版 算法 identifier。
func LegacyOutcomeMappingFromAlgorithm(algorithm modelcatalog.Algorithm) OutcomeMappingSpec {
	switch algorithm {
	case modelcatalog.AlgorithmBigFive:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailTraitProfile,
			DetailAdapterKey: DetailAdapterTraitProfile,
			Algorithm:        algorithm,
		}
	case modelcatalog.AlgorithmSBTI:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailPersonalityType,
			DetailAdapterKey: DetailAdapterPersonalityType,
			Algorithm:        algorithm,
		}
	default:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailPersonalityType,
			DetailAdapterKey: DetailAdapterPersonalityType,
			Algorithm:        algorithm,
		}
	}
}

// LegacyReportSpecFromPayload 推导report spec 从 旧版 载荷 算法 field。
func LegacyReportSpecFromPayload(p *Payload) ReportSpec {
	if p == nil {
		return ReportSpec{}
	}
	return LegacyReportSpecFromAlgorithm(p.Algorithm)
}

// LegacyReportSpecFromAlgorithm 推导report spec 从 旧版 算法 identifier。
func LegacyReportSpecFromAlgorithm(algorithm modelcatalog.Algorithm) ReportSpec {
	switch algorithm {
	case modelcatalog.AlgorithmBigFive:
		return ReportSpec{
			Kind:          ReportKindTraitProfile,
			AdapterKey:    ReportAdapterTraitProfile,
			CategoryLabel: "Big Five",
		}
	case modelcatalog.AlgorithmSBTI:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterPersonalityType,
			CategoryLabel: "SBTI",
		}
	default:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterPersonalityType,
			CategoryLabel: "MBTI",
		}
	}
}
