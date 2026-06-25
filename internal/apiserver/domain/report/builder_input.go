package report

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

// FactorScoreInput 因子得分输入。
type FactorScoreInput struct {
	FactorCode   FactorCode
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    RiskLevel
	Description  string
	Suggestion   string
	IsTotalScore bool
}
