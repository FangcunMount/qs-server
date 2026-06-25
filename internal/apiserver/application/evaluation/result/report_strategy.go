package result

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func resolveReportType(_ Outcome) domainReport.ReportType {
	return domainReport.ResolveReportType()
}
