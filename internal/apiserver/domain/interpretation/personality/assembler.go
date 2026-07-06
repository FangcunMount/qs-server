package personality

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"

// Build 组装人格类解读报告。
func Build(input Input) *domainreport.InterpretReport {
	return domainreport.NewInterpretReport(
		input.AssessmentID,
		input.Profile.ReportModelName(),
		input.Profile.ReportModelCode(input.ModelCode),
		input.TotalScore,
		input.RiskLevel,
		input.Conclusion,
		input.Dimensions,
		input.Suggestions,
		input.Profile.ModelExtra(),
	)
}
