package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
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

// NewSBTIReportBuilder is a characterization helper for the SBTI typology module.
func NewSBTIReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewReportBuilder(assessmentmodel.AlgorithmSBTI)
	if err != nil {
		panic(err)
	}
	return builder
}
