package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func resolveReportType(_ evaloutcome.Outcome) domainReport.ReportType {
	return domainReport.ResolveReportType()
}
