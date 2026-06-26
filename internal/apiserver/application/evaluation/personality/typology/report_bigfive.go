package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality/typology"
)

func buildBigFiveReport(outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	input, err := BigFiveReportInputFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	rpt, err := reporttypology.BuildBigFiveReport(input)
	if err != nil {
		return nil, err
	}
	return rpt, nil
}

func BigFiveResultDetailFromOutcome(outcome evaluationresult.Outcome) (evaluationtypology.BigFiveResultDetail, error) {
	if outcome.Execution == nil {
		return evaluationtypology.BigFiveResultDetail{}, fmt.Errorf("evaluation outcome is required")
	}
	return evaluationtypology.BigFiveResultDetailFromPayload(outcome.Execution.Detail.Payload)
}

// NewBigFiveReportBuilder is a characterization helper for the Big Five typology module.
func NewBigFiveReportBuilder() evaluationresult.ReportBuilder {
	builder, err := NewReportBuilder(assessmentmodel.AlgorithmBigFive)
	if err != nil {
		panic(err)
	}
	return builder
}
