package report

// DraftBuilder assembles report content without creating a report identity or
// lifecycle-bearing compatibility object.
type DraftBuilder interface {
	BuildDraft(input GenerateReportInput) (*Draft, error)
}

// GenerateReportInput 生成报告的输入参数。
// 由应用层从评估结果组装后传入。
type GenerateReportInput struct {
	AssessmentID ID
	ModelName    string
	ModelCode    string
	TotalScore   float64
	RiskLevel    RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []FactorScoreInput
}
