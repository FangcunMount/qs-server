package mbti

import (
	"testing"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func TestBuildReportFillsModelExtra(t *testing.T) {
	detail := ReportDetail{
		TypeCode:     "INTJ",
		TypeName:     "建筑师",
		OneLiner:     "独立战略家",
		ImageURL:     "https://example.com/intj.png",
		MatchPercent: 75,
		Dimensions: []DimensionReport{
			{
				Code:       "E_I",
				Name:       "外向-内向",
				RawScore:   31,
				Preference: "I",
				Strength:   78,
			},
		},
		Profile: ProfileReport{
			TypeCode:    "INTJ",
			TypeName:    "建筑师",
			Summary:     "善于长远规划",
			Strengths:   []string{"系统思考"},
			Weaknesses:  []string{"容易忽略情绪"},
			Suggestions: []string{"保留沟通余量"},
		},
		Source: SourceReport{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}

	report, err := BuildReport(ReportInput{
		AssessmentID: domainreport.ID(7001),
		ModelCode:    "MBTI_OEJTS",
		Detail:       detail,
	})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if report.ModelName() != "MBTI 人格类型测评 - 建筑师" {
		t.Fatalf("ModelName = %q", report.ModelName())
	}
	if report.ModelCode() != "MBTI_OEJTS" {
		t.Fatalf("ModelCode = %q", report.ModelCode())
	}
	if report.Conclusion() != "INTJ 建筑师 - 独立战略家" {
		t.Fatalf("Conclusion = %q", report.Conclusion())
	}
	dimensions := report.Dimensions()
	if len(dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(dimensions))
	}
	if dimensions[0].FactorCode() != domainreport.FactorCode("E_I") ||
		dimensions[0].FactorName() != "外向-内向" ||
		dimensions[0].RawScore() != 31 ||
		dimensions[0].RiskLevel() != domainreport.RiskLevelNone ||
		dimensions[0].Description() != "外向-内向：倾向 I（原始分 31，偏好强度 78%）" {
		t.Fatalf("unexpected dimension: %#v", dimensions[0])
	}
	if dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 40 {
		t.Fatalf("dimension MaxScore = %v, want 40", dimensions[0].MaxScore())
	}
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "善于长远规划")
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "优势：系统思考")
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "注意：容易忽略情绪")
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "建议：保留沟通余量")
	assertReportSuggestion(t, report.Suggestions(), domainreport.SuggestionCategoryGeneral, nil, "来源与授权：OEJTS；License: CC BY-NC-SA 4.0；非商业使用: true。")
	extra := report.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.Kind != "mbti" || extra.TypeCode != "INTJ" || extra.TypeName != "建筑师" {
		t.Fatalf("unexpected model extra: %#v", extra)
	}
	if extra.MatchPercent != 75 {
		t.Fatalf("MatchPercent = %v, want 75", extra.MatchPercent)
	}
	if extra.ImageURL != "https://example.com/intj.png" {
		t.Fatalf("ImageURL = %q", extra.ImageURL)
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
