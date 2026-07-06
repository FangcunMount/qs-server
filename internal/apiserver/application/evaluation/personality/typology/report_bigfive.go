package typology

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality/typology"
)

func buildBigFiveReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
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

func BigFiveResultDetailFromOutcome(outcome evaloutcome.Outcome) (evaluationtypology.BigFiveResultDetail, error) {
	if outcome.Execution == nil {
		return evaluationtypology.BigFiveResultDetail{}, fmt.Errorf("evaluation outcome is required")
	}
	return evaluationtypology.BigFiveResultDetailFromPayload(outcome.Execution.Detail.Payload)
}

// NewBigFiveReportBuilder is a characterization helper for typology reports.
func NewBigFiveReportBuilder() interpretationreporting.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
