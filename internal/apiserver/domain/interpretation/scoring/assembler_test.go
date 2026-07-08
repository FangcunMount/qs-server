package scoring_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
)

func TestBuildReportAssemblesScoreBasedInterpretReport(t *testing.T) {
	totalMax := 27.0
	got, err := scoring.BuildReport(builder.NewDefaultInterpretReportBuilder(nil), scoring.ReportInput{
		AssessmentID: report.ID(9001),
		Model: &scoring.ReportModel{
			Code:  "PHQ9",
			Title: "抑郁筛查",
			Factors: []scoring.FactorReportModel{
				{Code: "TOTAL", Title: "总分", MaxScore: &totalMax},
			},
		},
		TotalScore: 8,
		RiskLevel:  report.RiskLevelLow,
		Conclusion: "总体轻度风险",
		Suggestion: "持续观察整体状态",
		FactorScores: []scoring.FactorReportScore{
			{
				FactorCode:   "TOTAL",
				RawScore:     8,
				RiskLevel:    report.RiskLevelLow,
				Conclusion:   "总分提示轻度风险",
				Suggestion:   "保持规律作息",
				IsTotalScore: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if got.ModelName() != "抑郁筛查" || got.ModelCode() != "PHQ9" {
		t.Fatalf("model = %q/%q", got.ModelName(), got.ModelCode())
	}
	if got.TotalScore() != 8 || got.RiskLevel() != report.RiskLevelLow {
		t.Fatalf("summary = score:%v risk:%s", got.TotalScore(), got.RiskLevel())
	}
	if got.Conclusion() != "总分提示轻度风险" {
		t.Fatalf("Conclusion = %q", got.Conclusion())
	}
	dimensions := got.Dimensions()
	if len(dimensions) != 1 || dimensions[0].Name() != "总分" || dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != totalMax {
		t.Fatalf("unexpected dimensions: %#v", dimensions)
	}
}
