package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// LegacyOutcomeMappingFromAlgorithm 推导结果 mapping 从 旧版 算法 identifier。
func LegacyOutcomeMappingFromAlgorithm(algorithm binding.Algorithm) OutcomeMappingSpec {
	switch algorithm {
	case binding.AlgorithmBigFive:
		return OutcomeMappingSpec{
			DetailKind:       OutcomeDetailTraitProfile,
			DetailAdapterKey: DetailAdapterTraitProfile,
			Algorithm:        algorithm,
		}
	case binding.AlgorithmSBTI:
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
func LegacyReportSpecFromAlgorithm(algorithm binding.Algorithm) ReportSpec {
	switch algorithm {
	case binding.AlgorithmBigFive:
		return ReportSpec{
			Kind:          ReportKindTraitProfile,
			AdapterKey:    ReportAdapterTraitProfile,
			TemplateID:    "bigfive",
			CategoryLabel: "Big Five",
		}
	case binding.AlgorithmSBTI:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterPersonalityType,
			TemplateID:    "sbti",
			CategoryLabel: "SBTI",
		}
	default:
		return ReportSpec{
			Kind:          ReportKindPersonalityType,
			AdapterKey:    ReportAdapterPersonalityType,
			TemplateID:    "mbti",
			CategoryLabel: "MBTI",
		}
	}
}
