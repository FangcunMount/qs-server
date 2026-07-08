package patterns

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestBuildMBTIReportFillsModelExtra(t *testing.T) {
	detail := MBTIReportDetail{
		TypeCode:     "INTJ",
		TypeName:     "建筑师",
		OneLiner:     "独立战略家",
		ImageURL:     "https://example.com/intj.png",
		MatchPercent: 75,
		Dimensions: []MBTIDimensionReport{
			{
				Code:       "E_I",
				Name:       "外向-内向",
				RawScore:   31,
				Preference: "I",
				Strength:   78,
			},
		},
		Profile: MBTIProfileReport{
			TypeCode:    "INTJ",
			TypeName:    "建筑师",
			Summary:     "善于长远规划",
			Strengths:   []string{"系统思考"},
			Weaknesses:  []string{"容易忽略情绪"},
			Suggestions: []string{"保留沟通余量"},
		},
		Source: MBTISourceReport{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}

	got, err := BuildPersonalityTypeReport(PersonalityTypeReportInput{
		AssessmentID: report.ID(7001),
		ModelCode:    "MBTI_OEJTS",
		Detail:       mbtiMechanismDetail(detail),
	}, MBTIPersonalityTypeTemplate())
	if err != nil {
		t.Fatalf("BuildMBTIReport: %v", err)
	}
	if got.ModelName() != "MBTI 人格类型测评 - 建筑师" {
		t.Fatalf("ModelName = %q", got.ModelName())
	}
	if got.ModelCode() != "MBTI_OEJTS" {
		t.Fatalf("ModelCode = %q", got.ModelCode())
	}
	if got.Conclusion() != "INTJ 建筑师 - 独立战略家" {
		t.Fatalf("Conclusion = %q", got.Conclusion())
	}
	dimensions := got.Dimensions()
	if len(dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(dimensions))
	}
	if dimensions[0].Code() != report.NewDimensionCode("E_I") ||
		dimensions[0].Name() != "外向-内向" ||
		dimensions[0].RawScore() != 31 ||
		dimensions[0].Severity() != string(report.RiskLevelNone) ||
		dimensions[0].Description() != "外向-内向：倾向 I（原始分 31，偏好强度 78%）" {
		t.Fatalf("unexpected dimension: %#v", dimensions[0])
	}
	if dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 40 {
		t.Fatalf("dimension MaxScore = %v, want 40", dimensions[0].MaxScore())
	}
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "善于长远规划")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "优势：系统思考")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "注意：容易忽略情绪")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "建议：保留沟通余量")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "来源与授权：OEJTS；License: CC BY-NC-SA 4.0；非商业使用: true。")
	extra := got.ModelExtra()
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

func TestBuildSBTIReportSetsModelExtra(t *testing.T) {
	detail := SBTIReportDetail{
		TypeCode:   "CTRL",
		TypeName:   "拿捏者",
		OneLiner:   "人形自走任务管理器",
		Similarity: 0.92,
		ImageURL:   "https://example.com/CTRL.png",
		Rarity: SBTIRarityReport{
			Percent: 3.61,
			Label:   "中等",
			OneInX:  28,
		},
		Dimensions: []SBTIDimensionReport{
			{
				Code:     "SOCIAL",
				Name:     "社交姿态",
				Model:    "狗塑",
				RawScore: 5,
				Level:    "高",
			},
		},
		Outcome: SBTIOutcomeReport{
			Code:       "CTRL",
			Name:       "拿捏者",
			IsSpecial:  true,
			Commentary: "测试解读",
		},
		Source: SBTISourceReport{
			Attribution:   "SBTI Wiki",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
		SpecialTrigger: "全维度高匹配",
	}

	got, err := BuildPersonalityTypeReport(PersonalityTypeReportInput{
		AssessmentID: report.ID(7001),
		ModelCode:    "SBTI_FUN",
		Detail:       sbtiMechanismDetail(detail),
	}, SBTIPersonalityTypeTemplate())
	if err != nil {
		t.Fatalf("BuildSBTIReport: %v", err)
	}
	if got.ModelName() != "SBTI 趣味人格测评 - 拿捏者" {
		t.Fatalf("ModelName = %q", got.ModelName())
	}
	if got.ModelCode() != "SBTI_FUN" {
		t.Fatalf("ModelCode = %q", got.ModelCode())
	}
	if got.Conclusion() != "CTRL 拿捏者 - 人形自走任务管理器（匹配度 92%）" {
		t.Fatalf("Conclusion = %q", got.Conclusion())
	}
	dimensions := got.Dimensions()
	if len(dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(dimensions))
	}
	if dimensions[0].Code() != report.NewDimensionCode("SOCIAL") ||
		dimensions[0].Name() != "社交姿态" ||
		dimensions[0].RawScore() != 5 ||
		dimensions[0].Severity() != string(report.RiskLevelNone) ||
		dimensions[0].Description() != "狗塑 / 社交姿态：高 档，原始分 5/6" {
		t.Fatalf("unexpected dimension: %#v", dimensions[0])
	}
	if dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != 6 {
		t.Fatalf("dimension MaxScore = %v, want 6", dimensions[0].MaxScore())
	}
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "测试解读")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "来源与授权：SBTI Wiki；License: CC BY-NC-SA 4.0；非商业使用: true。")
	extra := got.ModelExtra()
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

func TestBuildBigFiveReportFillsTraitDimensions(t *testing.T) {
	detail := BigFiveReportDetail{
		Traits: []BigFiveTraitReport{
			{Code: "O", Name: "Openness", RawScore: 6},
			{Code: "C", Name: "Conscientiousness", RawScore: 8},
		},
		Source: BigFiveSourceReport{
			Attribution:   "IPIP",
			License:       "CC0",
			NonCommercial: false,
		},
	}

	got, err := BuildTraitProfileReport(TraitProfileReportInput{
		AssessmentID: report.ID(7001),
		ModelCode:    "BIGFIVE_V1",
		Detail:       bigFiveMechanismDetail(detail),
	}, BigFiveTraitProfileTemplate())
	if err != nil {
		t.Fatalf("BuildBigFiveReport: %v", err)
	}
	if got.ModelName() != "Big Five 五大人格特质测评 - 五大人格特质" {
		t.Fatalf("ModelName = %q", got.ModelName())
	}
	if got.ModelCode() != "BIGFIVE_V1" {
		t.Fatalf("ModelCode = %q", got.ModelCode())
	}
	if got.Conclusion() != "五大人格特质画像 - Openness 6 / Conscientiousness 8" {
		t.Fatalf("Conclusion = %q", got.Conclusion())
	}
	dimensions := got.Dimensions()
	if len(dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dimensions))
	}
	if dimensions[0].Code() != report.NewDimensionCode("O") ||
		dimensions[0].Name() != "Openness" ||
		dimensions[0].RawScore() != 6 ||
		dimensions[0].Kind() != report.DimensionKindTrait ||
		dimensions[0].Description() != "Openness：原始分 6" {
		t.Fatalf("unexpected dimension[0]: %#v", dimensions[0])
	}
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "特质分布：Openness 6 / Conscientiousness 8")
	assertReportSuggestion(t, got.Suggestions(), report.SuggestionCategoryGeneral, nil, "来源与授权：IPIP；License: CC0；非商业使用: false。")
	extra := got.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.Kind != "bigfive" || extra.TypeName != "五大人格特质" {
		t.Fatalf("unexpected model extra: %#v", extra)
	}
	if extra.Commentary != "Openness 6 / Conscientiousness 8" {
		t.Fatalf("Commentary = %q", extra.Commentary)
	}
}

func assertReportSuggestion(
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
