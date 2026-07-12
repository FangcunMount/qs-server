package scoring_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
)

func TestBuildFactorScoringDraftAssemblesInterpretReportContent(t *testing.T) {
	totalMax := 27.0
	sleepMax := 3.0
	got, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
		AssessmentID: report.ID(9001),
		Scale: &scoring.ReportModel{
			Code:  "PHQ9",
			Title: "抑郁筛查",
			Factors: []scoring.FactorReportModel{
				{Code: "TOTAL", Title: "总分", MaxScore: &totalMax},
				{Code: "SLEEP", Title: "睡眠", MaxScore: &sleepMax},
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
			{
				FactorCode: "SLEEP",
				RawScore:   2,
				RiskLevel:  report.RiskLevelMedium,
				Conclusion: "睡眠问题明显",
				Suggestion: "建立睡前放松流程",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildFactorScoringDraft: %v", err)
	}
	content := got.Content()
	if content.Model.Title != "抑郁筛查" {
		t.Fatalf("ModelName = %q", content.Model.Title)
	}
	if content.Model.Code != "PHQ9" {
		t.Fatalf("ModelCode = %q", content.Model.Code)
	}
	if content.PrimaryScore == nil || content.PrimaryScore.Value != 8 || content.Level == nil || content.Level.Code != string(report.RiskLevelLow) {
		t.Fatalf("summary = score:%#v level:%#v", content.PrimaryScore, content.Level)
	}
	if content.Conclusion != "总分提示轻度风险" {
		t.Fatalf("Conclusion = %q", content.Conclusion)
	}

	dimensions := content.Dimensions
	if len(dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dimensions))
	}
	if dimensions[0].Name() != "总分" || dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 27 {
		t.Fatalf("unexpected total dimension: %#v", dimensions[0])
	}
	if dimensions[1].Name() != "睡眠" ||
		dimensions[1].Severity() != string(report.RiskLevelMedium) ||
		dimensions[1].Description() != "睡眠问题明显" ||
		dimensions[1].Suggestion() != "建立睡前放松流程" {
		t.Fatalf("unexpected sleep dimension: %#v", dimensions[1])
	}

	assertScaleReportSuggestion(t, content.Suggestions, report.SuggestionCategoryGeneral, nil, "持续观察整体状态")
	assertScaleReportSuggestion(t, content.Suggestions, report.SuggestionCategoryGeneral, nil, "保持规律作息")
	sleepCode := report.FactorCode("SLEEP")
	assertScaleReportSuggestion(t, content.Suggestions, report.SuggestionCategoryDimension, &sleepCode, "建立睡前放松流程")
	if content.ModelExtra != nil {
		t.Fatalf("ModelExtra = %#v, want nil", content.ModelExtra)
	}
}

func assertScaleReportSuggestion(
	t *testing.T,
	suggestions []report.Suggestion,
	category report.SuggestionCategory,
	factorCode *report.FactorCode,
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
