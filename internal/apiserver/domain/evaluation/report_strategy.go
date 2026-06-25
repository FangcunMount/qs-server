package evaluation

import domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"

// ResolveReportType 选择报告展示策略。
// v1 仅支持默认 standard 报告；interpretation（评分/解读）与 report（持久化展示）在此分界：
// handler 产出 EvaluationResult，report builder 将其转为 InterpretReport。
func ResolveReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}
