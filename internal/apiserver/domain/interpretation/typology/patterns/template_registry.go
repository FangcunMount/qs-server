package patterns

// ReportSpec is the report-template selection input owned by the interpretation
// boundary. Runtime payload packages map into it at adapter edges.
type ReportSpec struct {
	AdapterKey ReportAdapterKey
	TemplateID string
}

// ReportAdapterKey selects a built-in report template adapter.
type ReportAdapterKey string

const (
	ReportAdapterPersonalityType ReportAdapterKey = "personality_type"
	ReportAdapterTraitProfile    ReportAdapterKey = "trait_profile"
	ReportAdapterMBTI            ReportAdapterKey = "mbti"
	ReportAdapterSBTI            ReportAdapterKey = "sbti"
	ReportAdapterBigFive         ReportAdapterKey = "bigfive"
)

// PersonalityTypeTemplateByID resolves a personality-type report template factory by TemplateID.
func PersonalityTypeTemplateByID(templateID string) (PersonalityTypeReportTemplate, bool) {
	switch templateID {
	case "mbti":
		return MBTIPersonalityTypeTemplate(), true
	case "sbti":
		return SBTIPersonalityTypeTemplate(), true
	default:
		return PersonalityTypeReportTemplate{}, false
	}
}

// TraitProfileTemplateByID resolves a trait-profile report template factory by TemplateID.
func TraitProfileTemplateByID(templateID string) (TraitProfileReportTemplate, bool) {
	switch templateID {
	case "bigfive":
		return BigFiveTraitProfileTemplate(), true
	default:
		return TraitProfileReportTemplate{}, false
	}
}

// PersonalityTypeTemplateForSpec resolves templates from report spec.
func PersonalityTypeTemplateForSpec(spec ReportSpec) PersonalityTypeReportTemplate {
	if tmpl, ok := PersonalityTypeTemplateByID(spec.TemplateID); ok {
		return tmpl
	}
	switch spec.AdapterKey {
	case ReportAdapterSBTI:
		return SBTIPersonalityTypeTemplate()
	case ReportAdapterMBTI:
		return MBTIPersonalityTypeTemplate()
	default:
		return PersonalityTypeReportTemplate{}
	}
}

// TraitProfileTemplateForSpec resolves templates from report spec.
func TraitProfileTemplateForSpec(spec ReportSpec) TraitProfileReportTemplate {
	if tmpl, ok := TraitProfileTemplateByID(spec.TemplateID); ok {
		return tmpl
	}
	switch spec.AdapterKey {
	case ReportAdapterBigFive:
		return BigFiveTraitProfileTemplate()
	default:
		return TraitProfileReportTemplate{}
	}
}
