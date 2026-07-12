package policy

// ReportType 报告模板类型；v1 仅支持 standard 默认报告。
type ReportType string

const ReportTypeStandard ReportType = "standard"

func (t ReportType) String() string {
	return string(t)
}

func (t ReportType) IsEmpty() bool {
	return t == ""
}

// TemplateVersion identifies one immutable release of all report-producing
// assets: template, builder behavior, interpretation rules and content schema.
// A new version produces a new ReportGeneration instead of overwriting a
// generated artifact.
type TemplateVersion string

// TemplateVersionV1 names the frozen compatibility release used by existing
// persisted outcomes until model-catalog publishes explicit template versions.
const TemplateVersionV1 TemplateVersion = "legacy-v1"

func (v TemplateVersion) String() string {
	return string(v)
}

func (v TemplateVersion) IsEmpty() bool {
	return v == ""
}
