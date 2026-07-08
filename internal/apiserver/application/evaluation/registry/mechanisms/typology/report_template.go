package typology

import (
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func personalityTypeTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.PersonalityTypeReportTemplate {
	switch spec.TemplateID {
	case "sbti":
		return reporttypology.SBTIPersonalityTypeTemplate()
	case "mbti":
		return reporttypology.MBTIPersonalityTypeTemplate()
	}
	switch spec.AdapterKey {
	case modeltypology.ReportAdapterSBTI:
		return reporttypology.SBTIPersonalityTypeTemplate()
	case modeltypology.ReportAdapterMBTI:
		return reporttypology.MBTIPersonalityTypeTemplate()
	default:
		return reporttypology.PersonalityTypeReportTemplate{}
	}
}

func traitProfileTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.TraitProfileReportTemplate {
	switch spec.TemplateID {
	case "bigfive":
		return reporttypology.BigFiveTraitProfileTemplate()
	}
	switch spec.AdapterKey {
	case modeltypology.ReportAdapterBigFive:
		return reporttypology.BigFiveTraitProfileTemplate()
	default:
		return reporttypology.TraitProfileReportTemplate{}
	}
}
