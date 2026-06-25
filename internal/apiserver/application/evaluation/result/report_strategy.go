package result

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func resolveReportType(_ Outcome) domainReport.ReportType {
	return evaluationdomain.ResolveReportType()
}
