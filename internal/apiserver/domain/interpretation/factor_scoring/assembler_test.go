package factor_scoring

import (
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestBuildReportAssemblesScoreBasedInterpretReport(t *testing.T) {
	totalMax := 27.0
	report, err := BuildReport(domainreport.NewDefaultInterpretReportBuilder(nil), ReportInput{
		AssessmentID: domainreport.ID(9001),
		Model: &ReportModel{
			Code:  "PHQ9",
			Title: "抑郁筛查",
			Factors: []FactorReportModel{
				{Code: "TOTAL", Title: "总分", MaxScore: &totalMax},
			},
		},
		TotalScore: 8,
		RiskLevel:  domainreport.RiskLevelLow,
		Conclusion: "总体轻度风险",
		Suggestion: "持续观察整体状态",
		FactorScores: []FactorReportScore{
			{
				FactorCode:   "TOTAL",
				RawScore:     8,
				RiskLevel:    domainreport.RiskLevelLow,
				Conclusion:   "总分提示轻度风险",
				Suggestion:   "保持规律作息",
				IsTotalScore: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.ModelName() != "抑郁筛查" || report.ModelCode() != "PHQ9" {
		t.Fatalf("model = %q/%q", report.ModelName(), report.ModelCode())
	}
	if report.TotalScore() != 8 || report.RiskLevel() != domainreport.RiskLevelLow {
		t.Fatalf("summary = score:%v risk:%s", report.TotalScore(), report.RiskLevel())
	}
	if report.Conclusion() != "总分提示轻度风险" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}
	dimensions := report.Dimensions()
	if len(dimensions) != 1 || dimensions[0].Name() != "总分" || dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != totalMax {
		t.Fatalf("unexpected dimensions: %#v", dimensions)
	}
}
