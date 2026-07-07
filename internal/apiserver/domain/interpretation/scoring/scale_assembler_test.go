package scoring

import (
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestBuildScaleReportAssemblesInterpretReport(t *testing.T) {
	totalMax := 27.0
	sleepMax := 3.0
	report, err := BuildScaleReport(domainreport.NewDefaultInterpretReportBuilder(nil), ScaleReportInput{
		AssessmentID: domainreport.ID(9001),
		Scale: &ReportModel{
			Code:  "PHQ9",
			Title: "抑郁筛查",
			Factors: []FactorReportModel{
				{Code: "TOTAL", Title: "总分", MaxScore: &totalMax},
				{Code: "SLEEP", Title: "睡眠", MaxScore: &sleepMax},
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
			{
				FactorCode: "SLEEP",
				RawScore:   2,
				RiskLevel:  domainreport.RiskLevelMedium,
				Conclusion: "睡眠问题明显",
				Suggestion: "建立睡前放松流程",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildScaleReport: %v", err)
	}
	if report.ModelName() != "抑郁筛查" {
		t.Fatalf("ModelName = %q", report.ModelName())
	}
	if report.ModelCode() != "PHQ9" {
		t.Fatalf("ModelCode = %q", report.ModelCode())
	}
	if report.TotalScore() != 8 || report.RiskLevel() != domainreport.RiskLevelLow {
		t.Fatalf("summary = score:%v risk:%s", report.TotalScore(), report.RiskLevel())
	}
	if report.Conclusion() != "总分提示轻度风险" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}

	dimensions := report.Dimensions()
	if len(dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dimensions))
	}
	if dimensions[0].Name() != "总分" || dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 27 {
		t.Fatalf("unexpected total dimension: %#v", dimensions[0])
	}
	if dimensions[1].Name() != "睡眠" ||
		dimensions[1].Severity() != string(domainreport.RiskLevelMedium) ||
		dimensions[1].Description() != "睡眠问题明显" ||
		dimensions[1].Suggestion() != "建立睡前放松流程" {
		t.Fatalf("unexpected sleep dimension: %#v", dimensions[1])
	}

	assertScaleReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "持续观察整体状态")
	assertScaleReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "保持规律作息")
	sleepCode := domainreport.FactorCode("SLEEP")
	assertScaleReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryDimension, &sleepCode, "建立睡前放松流程")
	if report.ModelExtra() != nil {
		t.Fatalf("ModelExtra = %#v, want nil", report.ModelExtra())
	}
}

func assertScaleReportSuggestion(
	t *testing.T,
	suggestions []domainreport.Suggestion,
	category domainreport.SuggestionCategory,
	factorCode *domainreport.FactorCode,
	content string,
) {
	t.Helper()
	for _, suggestion := range suggestions {
		if suggestion.Category != category || suggestion.Content != content {
			continue
		}
		if factorCode == nil && suggestion.FactorCode == nil {
			return
		}
		if factorCode != nil && suggestion.FactorCode != nil && *factorCode == *suggestion.FactorCode {
			return
		}
	}
	t.Fatalf("missing suggestion category=%s factor=%v content=%q in %#v", category, factorCode, content, suggestions)
}
