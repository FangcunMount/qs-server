package scoring

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestInterpretScaleFactorNoLastRuleFallback(t *testing.T) {
	t.Parallel()

	model := &ReportModel{Factors: []FactorReportModel{{
		Code:  "mood",
		Title: "情绪",
		InterpretRules: []FactorInterpretRule{
			{Min: 0, Max: 40, RiskLevel: "low", Conclusion: "偏低", Suggestion: "观察"},
			{Min: 40, Max: 60, RiskLevel: "medium", Conclusion: "中等", Suggestion: "关注", MaxInclusive: true},
		},
	}}}

	conclusionText, suggestion := interpretScaleFactor(model, FactorReportScore{
		FactorCode: "mood", FactorName: "情绪", RawScore: 80, RiskLevel: report.RiskLevelNone,
	})
	if conclusionText != "情绪得分80.0分，处于正常水平" {
		t.Fatalf("conclusion = %q, want default none-level wording (no last-rule fallback)", conclusionText)
	}
	if suggestion != "状态良好，继续保持" {
		t.Fatalf("suggestion = %q", suggestion)
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

	got, _ := interpretScaleFactor(model, FactorReportScore{FactorCode: "mood", RawScore: 100})
	if got != "high" {
		t.Fatalf("conclusion = %q, want high at max inclusive", got)
	}
	got, _ = interpretScaleFactor(model, FactorReportScore{FactorCode: "mood", RawScore: 40})
	if got != "high" {
		t.Fatalf("conclusion = %q, want high at boundary", got)
	}
}
