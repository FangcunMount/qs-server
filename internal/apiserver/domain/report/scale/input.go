package scale

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/score"
)

type FactorReportScore = reportscore.FactorReportScore

type ReportInput struct {
	AssessmentID domainreport.ID
	Scale        *ReportModel
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []FactorReportScore
}

type ReportModel = reportscore.ReportModel

type FactorReportModel = reportscore.FactorReportModel
