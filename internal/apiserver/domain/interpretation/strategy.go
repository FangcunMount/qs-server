package interpretation

// ReportType 报告模板类型；v1 仅支持 standard 默认报告。
type ReportType string

const ReportTypeStandard ReportType = "standard"

func (t ReportType) String() string {
	return string(t)
}

// ResolveReportType 选择报告展示策略。
// v1 仅支持默认 standard 报告；interpretation（评分/解读）与 report（持久化展示）在此分界。
func ResolveReportType() ReportType {
	return ReportTypeStandard
}
