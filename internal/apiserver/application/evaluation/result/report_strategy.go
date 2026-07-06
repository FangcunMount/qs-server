package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func resolveReportType(outcome Outcome) domainReport.ReportType {
	return interpretationreporting.OutcomeReportType(outcome)
}
