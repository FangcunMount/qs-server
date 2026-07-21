package scoring

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

func TestInterpretScaleFactorRequiresFrozenAssets(t *testing.T) {
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
		FactorCode: "mood", FactorName: "情绪", RawScore: 80,
		Level: &report.ResultLevel{Code: "high"},
	})
	if !errors.Is(err, ErrInterpretationAssetsMissing) {
		t.Fatalf("error = %v, want ErrInterpretationAssetsMissing", err)
	}
	if conclusionText != "" || suggestion != "" {
		t.Fatalf("presentation = (%q,%q), want empty without frozen assets", conclusionText, suggestion)
	}
}

func TestInterpretScaleFactorRequiresFrozenOutcomeCode(t *testing.T) {
	t.Parallel()

	assets := interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "high", Summary: "高风险"}}}
	_, _, err := interpretScaleFactor(&ReportModel{Assets: &assets}, FactorReportScore{
		FactorCode: "mood", RawScore: 80, RiskLevel: report.RiskLevelHigh,
	})
	if !errors.Is(err, ErrOutcomeCodeMissing) {
		t.Fatalf("error = %v, want ErrOutcomeCodeMissing", err)
	}
}

func TestInterpretScaleFactorDoesNotUseRiskLevelAsOutcomeCode(t *testing.T) {
	t.Parallel()

	assets := interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "high", Summary: "高风险"}}}
	_, _, err := interpretScaleFactor(&ReportModel{Assets: &assets}, FactorReportScore{
		FactorCode: "mood", RiskLevel: report.RiskLevelHigh,
	})
	if !errors.Is(err, ErrOutcomeCodeMissing) {
		t.Fatalf("error = %v, want ErrOutcomeCodeMissing", err)
	}
}
