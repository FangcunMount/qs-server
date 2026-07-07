package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func buildPersonalityTypeReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	switch legacyAlgorithmFromOutcome(outcome) {
	case modelcatalog.AlgorithmMBTI:
		input, err := MBTIReportInputFromOutcome(outcome)
		if err != nil {
			return nil, err
		}
		return reporttypology.BuildMBTIReport(input)
	case modelcatalog.AlgorithmSBTI:
		input, err := SBTIReportInputFromOutcome(outcome)
		if err != nil {
			return nil, err
		}
		return reporttypology.BuildSBTIReport(input)
	default:
		return buildGenericPersonalityTypeReport(outcome)
	}
}

func buildTraitProfileReport(_ modeltypology.ReportAdapterKey, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if legacyAlgorithmFromOutcome(outcome) == modelcatalog.AlgorithmBigFive {
		input, err := BigFiveReportInputFromOutcome(outcome)
		if err != nil {
			return nil, err
		}
		return reporttypology.BuildBigFiveReport(input)
	}
	return buildGenericTraitProfileReport(outcome)
}
