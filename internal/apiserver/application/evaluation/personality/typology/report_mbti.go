package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

func buildMBTIReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
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

// NewMBTIReportBuilder is a characterization helper for the MBTI typology module.
func NewMBTIReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewReportBuilder(assessmentmodel.AlgorithmMBTI)
	if err != nil {
		panic(err)
	}
	return builder
}
