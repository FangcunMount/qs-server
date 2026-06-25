package score

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"

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
	Code     string
	Title    string
	MaxScore *float64
}
