package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// OutcomeReportType selects the report type for an evaluation outcome.
func OutcomeReportType(_ evaloutcome.Outcome) domainReport.ReportType {
	return domainReport.ResolveReportType()
}

func resolveReportType(outcome evaloutcome.Outcome) domainReport.ReportType {
	return OutcomeReportType(outcome)
}
