package scoring

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestInterpretScaleFactorRuleMissFailsClosed(t *testing.T) {
	t.Parallel()

	model := &ReportModel{Factors: []FactorReportModel{{
		Code:  "mood",
		Title: "情绪",
		InterpretRules: []FactorInterpretRule{
			{Min: 0, Max: 40, RiskLevel: "low", Conclusion: "偏低", Suggestion: "观察"},
			{Min: 40, Max: 60, RiskLevel: "medium", Conclusion: "中等", Suggestion: "关注", MaxInclusive: true},
		},
	}}}

	conclusionText, suggestion, err := interpretScaleFactor(model, FactorReportScore{
		FactorCode: "mood", FactorName: "情绪", RawScore: 80, RiskLevel: report.RiskLevelNone,
	})
	if !errors.Is(err, ErrInterpretationRuleMiss) {
		t.Fatalf("error = %v, want ErrInterpretationRuleMiss", err)
	}
	if conclusionText != "" || suggestion != "" {
		t.Fatalf("presentation = (%q,%q), want empty on rule miss", conclusionText, suggestion)
	}
}

func TestInterpretScaleFactorMatchesSharedBounds(t *testing.T) {
	t.Parallel()

	model := &ReportModel{Factors: []FactorReportModel{{
		Code: "mood",
		InterpretRules: []FactorInterpretRule{
			{Min: 0, Max: 40, Conclusion: "low"},
			{Min: 40, Max: 100, Conclusion: "high", MaxInclusive: true},
		},
	}}}

	got, _, err := interpretScaleFactor(model, FactorReportScore{FactorCode: "mood", RawScore: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got != "high" {
		t.Fatalf("conclusion = %q, want high at max inclusive", got)
	}
	got, _, err = interpretScaleFactor(model, FactorReportScore{FactorCode: "mood", RawScore: 40})
	if err != nil {
		t.Fatal(err)
	}
	if got != "high" {
		t.Fatalf("conclusion = %q, want high at boundary", got)
	}
}

func TestInterpretScaleFactorSoftDefaultsOnlyWithoutRulesOrAssets(t *testing.T) {
	t.Parallel()

	conclusion, suggestion, err := interpretScaleFactor(
		&ReportModel{Factors: []FactorReportModel{{Code: "mood", Title: "情绪"}}},
		FactorReportScore{FactorCode: "mood", FactorName: "情绪", RawScore: 80},
	)
	if err != nil {
		t.Fatal(err)
	}
	if conclusion != "情绪得分80.0分，处于正常水平" || suggestion != "状态良好，继续保持" {
		t.Fatalf("soft default = (%q,%q)", conclusion, suggestion)
	}
}
