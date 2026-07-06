package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality/typology"
)

func buildMBTIReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	input, err := MBTIReportInputFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	rpt, err := reporttypology.BuildMBTIReport(input)
	if err != nil {
		return nil, err
	}
	return rpt, nil
}

// NewMBTIReportBuilder is a characterization helper for typology reports.
func NewMBTIReportBuilder() interpretationreporting.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
