package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

func buildSBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	input, err := SBTIReportInputFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	rpt, err := reporttypology.BuildSBTIReport(input)
	if err != nil {
		return nil, err
	}
	return rpt, nil
}

// NewSBTIReportBuilder is a characterization helper for typology reports.
func NewSBTIReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
