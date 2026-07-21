package scoring

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// MC-R016: Interpretation resolves presentation by OutcomeCode when assets are
// available, without re-matching score ranges for the decision fact itself.
func TestPresentationByOutcomeCodeDoesNotRequireScoreRematch(t *testing.T) {
	t.Parallel()
	assets := interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{
			OutcomeCode: "low", Title: "低风险", Summary: "偏低", Description: "建议观察",
		}},
	}
	model := &ReportModel{
		Code:   "SCL",
		Assets: &assets,
		Factors: []FactorReportModel{{
			Code: "mood",
			InterpretRules: []FactorInterpretRule{
				// Legacy rule would match score 80 as high, not low.
				{Min: 0, Max: 50, RiskLevel: "high", Conclusion: "wrong-high", MaxInclusive: true},
				{Min: 50, Max: 100, RiskLevel: "low", Conclusion: "wrong-low", MaxInclusive: true},
			},
		}},
	}
	fs := FactorReportScore{
		FactorCode: "mood", FactorName: "情绪", RawScore: 80, RiskLevel: report.RiskLevelLow,
		Level: &report.ResultLevel{Code: "low"},
	}
	conclusion, suggestion, err := interpretScaleFactor(model, fs)
	if err != nil {
		t.Fatal(err)
	}
	if conclusion != "偏低" || suggestion != "建议观察" {
		t.Fatalf("presentation = (%q,%q), want OutcomeCode lookup", conclusion, suggestion)
	}
	if fs.RiskLevel != report.RiskLevelLow {
		t.Fatal("decision RiskLevel must remain the frozen fact")
	}
}

func TestPresentationByOutcomeCodeRewritesCopyWithoutChangingDecisionFact(t *testing.T) {
	t.Parallel()
	assetsA := interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{
			OutcomeCode: "ability_high", Summary: "优秀", Description: "继续保持",
		}},
	}
	assetsB := interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{
			OutcomeCode: "ability_high", Summary: "能力较强（改写）", Description: "持续关注",
		}},
	}
	fs := FactorReportScore{FactorCode: "total", RawScore: 42, Level: &report.ResultLevel{Code: "ability_high"}}
	gotA, _, err := interpretScaleFactor(&ReportModel{Assets: &assetsA}, fs)
	if err != nil {
		t.Fatal(err)
	}
	gotB, _, err := interpretScaleFactor(&ReportModel{Assets: &assetsB}, fs)
	if err != nil {
		t.Fatal(err)
	}
	if gotA == gotB {
		t.Fatalf("presentation should differ after assets rewrite: %q vs %q", gotA, gotB)
	}
	if fs.Level.Code != "ability_high" {
		t.Fatalf("OutcomeCode fact changed: %#v", fs.Level)
	}
}

func TestPresentationDoesNotFallBackToScoreRematchWithoutAssets(t *testing.T) {
	t.Parallel()
	model := &ReportModel{
		Factors: []FactorReportModel{{
			Code: "total",
			InterpretRules: []FactorInterpretRule{
				{Min: 0, Max: 10, MaxInclusive: true, Conclusion: "legacy-conclusion", Suggestion: "legacy-suggestion"},
			},
		}},
	}
	conclusion, suggestion, err := interpretScaleFactor(model, FactorReportScore{
		FactorCode: "total", RawScore: 5, RiskLevel: report.RiskLevelLow,
		Level: &report.ResultLevel{Code: "low"},
	})
	if !errors.Is(err, ErrInterpretationAssetsMissing) {
		t.Fatalf("error = %v, want ErrInterpretationAssetsMissing", err)
	}
	if conclusion != "" || suggestion != "" {
		t.Fatalf("fallback presentation = (%q,%q), want empty", conclusion, suggestion)
	}
}
