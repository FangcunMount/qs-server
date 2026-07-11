package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
)

// OutcomeReportType 选择report type 用于 评估 结果。
func OutcomeReportType(_ evaloutcome.Outcome) domainReport.ReportType {
	return domainReport.ResolveReportType()
}

func resolveReportType(outcome evaloutcome.Outcome) domainReport.ReportType {
	return OutcomeReportType(outcome)
}
