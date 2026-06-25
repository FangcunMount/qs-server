package sbti

import (
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func TestBuildReportSetsModelExtra(t *testing.T) {
	detail := ReportDetail{
		TypeCode:   "CTRL",
		TypeName:   "拿捏者",
		OneLiner:   "人形自走任务管理器",
		Similarity: 0.92,
		ImageURL:   "https://example.com/CTRL.png",
		Rarity: RarityReport{
			Percent: 3.61,
			Label:   "中等",
			OneInX:  28,
		},
		Dimensions: []DimensionReport{
			{
				Code:     "SOCIAL",
				Name:     "社交姿态",
				Model:    "狗塑",
				RawScore: 5,
				Level:    "高",
			},
		},
		Outcome: OutcomeReport{
			Code:       "CTRL",
			Name:       "拿捏者",
			IsSpecial:  true,
			Commentary: "测试解读",
		},
		Source: SourceReport{
			Attribution:   "SBTI Wiki",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
		SpecialTrigger: "全维度高匹配",
	}

	report, err := BuildReport(ReportInput{
		AssessmentID: domainreport.ID(7001),
		ModelCode:    "SBTI_FUN",
		Detail:       detail,
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.ModelName() != "SBTI 趣味人格测评 - 拿捏者" {
		t.Fatalf("ModelName = %q", report.ModelName())
	}
	if report.ModelCode() != "SBTI_FUN" {
		t.Fatalf("ModelCode = %q", report.ModelCode())
	}
	if report.Conclusion() != "CTRL 拿捏者 - 人形自走任务管理器（匹配度 92%）" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}
	dimensions := report.Dimensions()
	if len(dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(dimensions))
	}
	if dimensions[0].FactorCode() != domainreport.FactorCode("SOCIAL") ||
		dimensions[0].FactorName() != "社交姿态" ||
		dimensions[0].RawScore() != 5 ||
		dimensions[0].RiskLevel() != domainreport.RiskLevelNone ||
		dimensions[0].Description() != "狗塑 / 社交姿态：高 档，原始分 5/6" {
		t.Fatalf("unexpected dimension: %#v", dimensions[0])
	}
	if dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 6 {
		t.Fatalf("dimension MaxScore = %v, want 6", dimensions[0].MaxScore())
	}
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "测试解读")
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "来源与授权：SBTI Wiki；License: CC BY-NC-SA 4.0；非商业使用: true。")
	extra := report.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.TypeCode != "CTRL" {
		t.Fatalf("TypeCode = %s, want CTRL", extra.TypeCode)
	}
	if extra.ImageURL == "" {
		t.Fatal("expected image url")
	}
	if extra.Rarity == nil || extra.Rarity.OneInX != 28 {
		t.Fatalf("rarity = %#v, want one_in_x 28", extra.Rarity)
	}
	if extra.MatchPercent != 92 {
		t.Fatalf("MatchPercent = %.2f, want 92", extra.MatchPercent)
	}
	if extra.Kind != "sbti" {
		t.Fatalf("Kind = %s, want sbti", extra.Kind)
	}
	if !extra.IsSpecial || extra.SpecialTrigger != "全维度高匹配" {
		t.Fatalf("special fields = isSpecial:%t trigger:%q", extra.IsSpecial, extra.SpecialTrigger)
	}
}

func assertReportSuggestion(
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
