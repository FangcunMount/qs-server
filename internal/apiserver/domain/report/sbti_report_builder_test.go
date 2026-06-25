package report

import (
	"testing"

	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
)

func TestBuildSBTIReportSetsModelExtra(t *testing.T) {
	detail := SBTIReportDetail{
		TypeCode:   "CTRL",
		TypeName:   "拿捏者",
		OneLiner:   "人形自走任务管理器",
		Similarity: 0.92,
		ImageURL:   "https://example.com/CTRL.png",
		Rarity: rulesetsbti.RaritySnapshot{
			Percent: 3.61,
			Label:   "中等",
			OneInX:  28,
		},
		Outcome: rulesetsbti.OutcomeSnapshot{
			Code:       "CTRL",
			Name:       "拿捏者",
			Commentary: "测试解读",
		},
	}

	report, err := BuildSBTIReport(SBTIReportInput{
		AssessmentID: ID(7001),
		ModelCode:    "SBTI_FUN",
		Detail:       detail,
	})
	if err != nil {
		t.Fatalf("BuildSBTIReport: %v", err)
	}
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
}
