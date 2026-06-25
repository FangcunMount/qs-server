package report

// ResolveReportType 选择报告展示策略。
// v1 仅支持默认 standard 报告；interpretation（评分/解读）与 report（持久化展示）在此分界。
func ResolveReportType() ReportType {
	return ReportTypeStandard
}
