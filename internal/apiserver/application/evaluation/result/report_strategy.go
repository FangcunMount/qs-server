package result

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func resolveReportType(_ Outcome) domainReport.ReportType {
	return evaluationdomain.ResolveReportType()
}
