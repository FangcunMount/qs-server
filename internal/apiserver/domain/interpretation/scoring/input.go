package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

type FactorReportScore struct {
	FactorCode     string
	FactorName     string
	RawScore       float64
	RiskLevel      report.RiskLevel
	DerivedScores  []report.ScoreValue
	Level          *report.ResultLevel
	NormReference  *report.NormReference
	Conclusion     string
	Suggestion     string
	IsTotalScore   bool
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
}

type ReportInput struct {
	AssessmentID report.ID
	Model        *ReportModel
	TotalScore   float64
	RiskLevel    report.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []FactorReportScore
}

type ReportModel struct {
	Code    string
	Title   string
	Factors []FactorReportModel
	Assets  *interpretationassets.Assets
}

type FactorReportModel struct {
	Code           string
	Title          string
	MaxScore       *float64
	IsTotalScore   bool
	InterpretRules []FactorInterpretRule
}

// FactorInterpretRule 是量表因子解读规则的中立表示，用于在解读侧生成结论/建议文案。
type FactorInterpretRule struct {
	Min          float64
	Max          float64
	MaxInclusive bool
	UnboundedMax bool
	RiskLevel    string
	Conclusion   string
	Suggestion   string
}

// Matches 使用与 Decision ScoreRange 相同的端点契约（默认半开区间）。
func (r FactorInterpretRule) Matches(score float64) bool {
	return scorerange.Bound{
		Min: r.Min, Max: r.Max, MaxInclusive: r.MaxInclusive, UnboundedMax: r.UnboundedMax,
	}.Contains(score)
}
