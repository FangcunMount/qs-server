package typology

import (
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func personalityTypeTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.PersonalityTypeReportTemplate {
	return reporttypology.PersonalityTypeTemplateForSpec(reportTemplateSpec(spec))
}

func traitProfileTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.TraitProfileReportTemplate {
	return reporttypology.TraitProfileTemplateForSpec(reportTemplateSpec(spec))
}

func reportTemplateSpec(spec modeltypology.ReportSpec) reporttypology.ReportSpec {
	return reporttypology.ReportSpec{
		AdapterKey: reporttypology.ReportAdapterKey(spec.AdapterKey),
		TemplateID: spec.TemplateID,
	}
}
