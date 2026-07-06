package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func AttachReportOutcomeSummary(outcome Outcome, report *domainreport.InterpretReport) *domainreport.InterpretReport {
	return interpretationreporting.AttachReportOutcomeSummary(outcome, report)
}
