package assessment

import (
	"testing"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func TestToModelExtraResultMapsPersonalityFields(t *testing.T) {
	got := toModelExtraResult(&report.ModelExtra{
		Kind:         "sbti",
		TypeCode:     "HIGH",
		TypeName:     "高能者",
		OneLiner:     "all high",
		MatchPercent: 100,
		Rarity: &report.ModelRarity{
			Percent: 12.5,
			Label:   "稀有",
			OneInX:  8,
		},
	})
	if got == nil {
		t.Fatal("expected model extra result")
	}
	if got.Kind != "sbti" || got.TypeCode != "HIGH" || got.MatchPercent != 100 {
		t.Fatalf("model extra = %#v", got)
	}
	if got.Rarity == nil || got.Rarity.Percent != 12.5 || got.Rarity.OneInX != 8 {
		t.Fatalf("rarity = %#v", got.Rarity)
	}
}

func TestToModelExtraResultReturnsNilForEmptyExtra(t *testing.T) {
	if got := toModelExtraResult(&report.ModelExtra{}); got != nil {
		t.Fatalf("model extra = %#v, want nil", got)
	}
	if got := toModelExtraResult(nil); got != nil {
		t.Fatalf("model extra = %#v, want nil", got)
	}
}

func TestReportRowToResultMapsModelExtra(t *testing.T) {
	got := reportRowToResult(evaluationreadmodel.ReportRow{
		AssessmentID: 303,
		ModelCode:    "SBTI_FUN",
		ModelExtra: &evaluationreadmodel.ReportModelExtraRow{
			Kind:           "sbti",
			TypeCode:       "HIGH",
			TypeName:       "高能者",
			IsSpecial:      false,
			SpecialTrigger: "",
			Commentary:     "commentary",
		},
	})
	if got == nil || got.ModelExtra == nil {
		t.Fatal("expected model extra on report result")
	}
	if got.ModelExtra.Kind != "sbti" || got.ModelExtra.TypeCode != "HIGH" || got.ModelExtra.Commentary != "commentary" {
		t.Fatalf("model extra = %#v", got.ModelExtra)
	}
}
