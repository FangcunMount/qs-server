package patterns

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"

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

// PersonalityTypeTemplateForSpec resolves templates from payload-first report spec.
func PersonalityTypeTemplateForSpec(spec modeltypology.ReportSpec) PersonalityTypeReportTemplate {
	if tmpl, ok := PersonalityTypeTemplateByID(spec.TemplateID); ok {
		return tmpl
	}
	switch spec.AdapterKey {
	case modeltypology.ReportAdapterSBTI:
		return SBTIPersonalityTypeTemplate()
	case modeltypology.ReportAdapterMBTI:
		return MBTIPersonalityTypeTemplate()
	default:
		return PersonalityTypeReportTemplate{}
	}
}

// TraitProfileTemplateForSpec resolves templates from payload-first report spec.
func TraitProfileTemplateForSpec(spec modeltypology.ReportSpec) TraitProfileReportTemplate {
	if tmpl, ok := TraitProfileTemplateByID(spec.TemplateID); ok {
		return tmpl
	}
	switch spec.AdapterKey {
	case modeltypology.ReportAdapterBigFive:
		return BigFiveTraitProfileTemplate()
	default:
		return TraitProfileReportTemplate{}
	}
}
