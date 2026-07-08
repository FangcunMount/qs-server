package policy

// ReportType 报告模板类型；v1 仅支持 standard 默认报告。
type ReportType string

const ReportTypeStandard ReportType = "standard"

func (t ReportType) String() string {
	return string(t)
}

// ResolveReportType 选择报告展示策略。
func ResolveReportType() ReportType {
	return ReportTypeStandard
}
