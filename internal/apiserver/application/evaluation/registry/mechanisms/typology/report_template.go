package typology

import (
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func personalityTypeTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.PersonalityTypeReportTemplate {
	return reporttypology.PersonalityTypeTemplateForSpec(spec)
}

func traitProfileTemplateForSpec(spec modeltypology.ReportSpec) reporttypology.TraitProfileReportTemplate {
	return reporttypology.TraitProfileTemplateForSpec(spec)
}
