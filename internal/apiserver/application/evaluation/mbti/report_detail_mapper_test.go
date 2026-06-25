package mbti

import (
	"testing"

	evaluationmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/mbti"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
)

func TestMBTIReportDetailMapperPreservesAllReportFields(t *testing.T) {
	detail := evaluationmbti.ResultDetail{
		TypeCode:     "INTJ",
		TypeName:     "建筑师",
		OneLiner:     "独立战略家",
		MatchPercent: 87.5,
		ImageURL:     "https://example.com/intj.png",
		Dimensions: []evaluationmbti.DimensionResult{
			{
				Code:       "EI",
				Name:       "外向-内向",
				LeftPole:   "E",
				RightPole:  "I",
				RawScore:   31,
				Preference: "I",
				Strength:   58,
			},
		},
		Profile: rulesetmbti.TypeProfileSnapshot{
			TypeCode:    "INTJ",
			TypeName:    "建筑师",
			OneLiner:    "独立战略家",
			Summary:     "善于长远规划",
			Traits:      []string{"独立", "理性"},
			Strengths:   []string{"系统思考"},
			Weaknesses:  []string{"表达克制"},
			Suggestions: []string{"保留沟通空间"},
			ImageURL:    "https://example.com/profile-intj.png",
		},
		Source: rulesetmbti.SourceSnapshot{
			QuestionsRepo: "serenakeyitan/mbti",
			SourceSite:    "https://example.com/mbti",
			License:       "CC BY-NC-SA 4.0",
			Attribution:   "OEJTS",
			NonCommercial: true,
		},
	}

	got := mbtiReportDetail(detail)

	if got.TypeCode != detail.TypeCode || got.TypeName != detail.TypeName ||
		got.OneLiner != detail.OneLiner || got.MatchPercent != detail.MatchPercent ||
		got.ImageURL != detail.ImageURL {
		t.Fatalf("basic fields = %#v, want %#v", got, detail)
	}
	if len(got.Dimensions) != 1 {
		t.Fatalf("dimensions len = %d, want 1", len(got.Dimensions))
	}
	dim := got.Dimensions[0]
	if dim.Code != "EI" || dim.Name != "外向-内向" || dim.LeftPole != "E" ||
		dim.RightPole != "I" || dim.RawScore != 31 || dim.Preference != "I" ||
		dim.Strength != 58 {
		t.Fatalf("dimension = %#v, want source dimension preserved", dim)
	}
	if got.Profile.TypeCode != "INTJ" || got.Profile.Summary != "善于长远规划" ||
		len(got.Profile.Traits) != 2 || got.Profile.Traits[1] != "理性" ||
		len(got.Profile.Strengths) != 1 || got.Profile.Strengths[0] != "系统思考" ||
		len(got.Profile.Weaknesses) != 1 || got.Profile.Weaknesses[0] != "表达克制" ||
		len(got.Profile.Suggestions) != 1 || got.Profile.Suggestions[0] != "保留沟通空间" ||
		got.Profile.ImageURL != "https://example.com/profile-intj.png" {
		t.Fatalf("profile = %#v, want source profile preserved", got.Profile)
	}
	if got.Source.QuestionsRepo != "serenakeyitan/mbti" ||
		got.Source.SourceSite != "https://example.com/mbti" ||
		got.Source.License != "CC BY-NC-SA 4.0" ||
		got.Source.Attribution != "OEJTS" ||
		!got.Source.NonCommercial {
		t.Fatalf("source = %#v, want source metadata preserved", got.Source)
	}
}
