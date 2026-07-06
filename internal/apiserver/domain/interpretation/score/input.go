package score

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"

type FactorReportScore struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    domainreport.RiskLevel
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

type ReportInput struct {
	AssessmentID domainreport.ID
	Model        *ReportModel
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []FactorReportScore
}

type ReportModel struct {
	Code    string
	Title   string
	Factors []FactorReportModel
}

type FactorReportModel struct {
	Code           string
	Title          string
	MaxScore       *float64
	InterpretRules []FactorInterpretRule
}

// FactorInterpretRule 是量表因子解读规则的中立表示，用于在解读侧生成结论/建议文案。
type FactorInterpretRule struct {
	Min        float64
	Max        float64
	RiskLevel  string
	Conclusion string
	Suggestion string
}

// Matches 采用左闭右开区间语义：score ∈ [Min, Max)。
func (r FactorInterpretRule) Matches(score float64) bool {
	return score >= r.Min && score < r.Max
}
