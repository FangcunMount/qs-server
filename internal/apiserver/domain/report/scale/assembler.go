package scale

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/score"
)

func BuildReport(composer domainreport.ReportBuilder, input ReportInput) (*domainreport.InterpretReport, error) {
	return reportscore.BuildReport(composer, reportscore.ReportInput{
		AssessmentID: input.AssessmentID,
		Model:        input.Scale,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   input.Conclusion,
		Suggestion:   input.Suggestion,
		FactorScores: input.FactorScores,
	})
}
