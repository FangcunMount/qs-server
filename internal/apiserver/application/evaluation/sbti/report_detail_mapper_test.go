package sbti

import (
	"testing"

	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/sbti"
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
)

func TestSBTIReportDetailMapperPreservesAllReportFields(t *testing.T) {
	detail := evaluationsbti.ResultDetail{
		TypeCode:   "CTRL",
		TypeName:   "拿捏者",
		OneLiner:   "人形自走任务管理器",
		Pattern:    "H-H-M-L",
		Similarity: 0.92,
		ImageURL:   "https://example.com/ctrl.png",
		Rarity: rulesetsbti.RaritySnapshot{
			Percent: 3.61,
			Label:   "中等",
			OneInX:  28,
		},
		Dimensions: []evaluationsbti.DimensionResult{
			{
				Code:     "A",
				Name:     "行动力",
				Model:    "Alpha",
				RawScore: 6,
				Level:    "H",
			},
		},
		Outcome: rulesetsbti.OutcomeSnapshot{
			Code:       "CTRL",
			Name:       "拿捏者",
			OneLiner:   "人形自走任务管理器",
			Pattern:    "H-H-M-L",
			Image:      "https://example.com/outcome.png",
			Rarity:     rulesetsbti.RaritySnapshot{Percent: 3.61, Label: "中等", OneInX: 28},
			IsSpecial:  true,
			Trigger:    "drink",
			Commentary: "测试解读",
		},
		Source: rulesetsbti.SourceSnapshot{
			WikiRepo:      "serenakeyitan/sbti-wiki",
			SourceSite:    "https://example.com/sbti",
			License:       "CC BY-NC-SA 4.0",
			Attribution:   "SBTI Wiki",
			ImageBaseURL:  "https://example.com/images",
			NonCommercial: true,
		},
		SpecialTrigger: "DRUNK",
	}

	got := sbtiReportDetail(detail)

	if got.TypeCode != detail.TypeCode || got.TypeName != detail.TypeName ||
		got.OneLiner != detail.OneLiner || got.Pattern != detail.Pattern ||
		got.Similarity != detail.Similarity || got.ImageURL != detail.ImageURL ||
		got.SpecialTrigger != detail.SpecialTrigger {
		t.Fatalf("basic fields = %#v, want %#v", got, detail)
	}
	if got.Rarity.Percent != 3.61 || got.Rarity.Label != "中等" || got.Rarity.OneInX != 28 {
		t.Fatalf("rarity = %#v, want source rarity preserved", got.Rarity)
	}
	if len(got.Dimensions) != 1 {
		t.Fatalf("dimensions len = %d, want 1", len(got.Dimensions))
	}
	dim := got.Dimensions[0]
	if dim.Code != "A" || dim.Name != "行动力" || dim.Model != "Alpha" ||
		dim.RawScore != 6 || dim.Level != "H" {
		t.Fatalf("dimension = %#v, want source dimension preserved", dim)
	}
	if got.Outcome.Code != "CTRL" || got.Outcome.Name != "拿捏者" ||
		got.Outcome.Pattern != "H-H-M-L" || got.Outcome.Image != "https://example.com/outcome.png" ||
		!got.Outcome.IsSpecial || got.Outcome.Trigger != "drink" ||
		got.Outcome.Commentary != "测试解读" ||
		got.Outcome.Rarity.Percent != 3.61 || got.Outcome.Rarity.OneInX != 28 {
		t.Fatalf("outcome = %#v, want source outcome preserved", got.Outcome)
	}
	if got.Source.WikiRepo != "serenakeyitan/sbti-wiki" ||
		got.Source.SourceSite != "https://example.com/sbti" ||
		got.Source.License != "CC BY-NC-SA 4.0" ||
		got.Source.Attribution != "SBTI Wiki" ||
		got.Source.ImageBaseURL != "https://example.com/images" ||
		!got.Source.NonCommercial {
		t.Fatalf("source = %#v, want source metadata preserved", got.Source)
	}
}
