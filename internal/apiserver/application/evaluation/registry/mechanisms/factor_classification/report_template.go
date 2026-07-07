package factor_classification

import (
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/factor_classification/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func personalityTypeTemplateForAlgorithm(algorithm modelcatalog.Algorithm) reporttypology.PersonalityTypeReportTemplate {
	switch algorithm {
	case modelcatalog.AlgorithmMBTI:
		return reporttypology.MBTIPersonalityTypeTemplate()
	case modelcatalog.AlgorithmSBTI:
		return reporttypology.SBTIPersonalityTypeTemplate()
	default:
		return reporttypology.PersonalityTypeReportTemplate{}
	}
}

func traitProfileTemplateForAlgorithm(algorithm modelcatalog.Algorithm) reporttypology.TraitProfileReportTemplate {
	switch algorithm {
	case modelcatalog.AlgorithmBigFive:
		return reporttypology.BigFiveTraitProfileTemplate()
	default:
		return reporttypology.TraitProfileReportTemplate{}
	}
}
