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

const (
	// TemplateVersionV1 names the frozen compatibility release used by retained
	// historical outcomes and reports.
	TemplateVersionV1 TemplateVersion = "legacy-v1"
	// TemplateVersionCurrent is the first explicitly published report semantics
	// release selected by governed ModelCatalog snapshots.
	TemplateVersionCurrent TemplateVersion = "2026-08-v1"
)

func (v TemplateVersion) String() string {
	return string(v)
}

func (v TemplateVersion) IsEmpty() bool {
	return v == ""
}
