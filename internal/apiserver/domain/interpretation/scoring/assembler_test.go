package scoring_test

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
)

func TestBuildFactorScoringDraftAssemblesScoreBasedContent(t *testing.T) {
	totalMax := 27.0
	got, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
		AssessmentID: report.ID(9001),
		Scale: &scoring.ReportModel{
			Code:  "PHQ9",
			Title: "抑郁筛查",
			Factors: []scoring.FactorReportModel{
				{Code: "TOTAL", Title: "总分", MaxScore: &totalMax},
			},
		},
		TotalScore: 8,
		RiskLevel:  report.RiskLevelLow,
		Conclusion: "总体轻度风险",
		Suggestion: "持续观察整体状态",
		FactorScores: []scoring.FactorReportScore{
			{
				FactorCode:   "TOTAL",
				RawScore:     8,
				RiskLevel:    report.RiskLevelLow,
				Conclusion:   "总分提示轻度风险",
				Suggestion:   "保持规律作息",
				IsTotalScore: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildFactorScoringDraft: %v", err)
	}
	content := got.Content()
	if content.Model.Title != "抑郁筛查" || content.Model.Code != "PHQ9" {
		t.Fatalf("model = %#v", content.Model)
	}
	if content.PrimaryScore == nil || content.PrimaryScore.Value != 8 || content.Level == nil || content.Level.Code != string(report.RiskLevelLow) {
		t.Fatalf("summary = score:%#v level:%#v", content.PrimaryScore, content.Level)
	}
	if content.Conclusion != "总分提示轻度风险" {
		t.Fatalf("Conclusion = %q", content.Conclusion)
	}
	dimensions := content.Dimensions
	if len(dimensions) != 1 || dimensions[0].Name() != "总分" || dimensions[0].MaxScore() == nil || *dimensions[0].MaxScore() != totalMax {
		t.Fatalf("unexpected dimensions: %#v", dimensions)
	}
}

func TestBuildFactorScoringDraftFailsWithoutFrozenInterpretationAssets(t *testing.T) {
	_, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
		AssessmentID: report.ID(9002),
		Scale: &scoring.ReportModel{Factors: []scoring.FactorReportModel{{
			Code: "TOTAL",
			InterpretRules: []scoring.FactorInterpretRule{{
				Min: 0, Max: 10, Conclusion: "low",
			}},
		}}},
		FactorScores: []scoring.FactorReportScore{{FactorCode: "TOTAL", RawScore: 20}},
	})
	if !errors.Is(err, scoring.ErrInterpretationAssetsMissing) {
		t.Fatalf("error = %v, want ErrInterpretationAssetsMissing", err)
	}
}

func TestBuildFactorScoringDraftRetainsHiddenFactorWithoutResolvingPresentation(t *testing.T) {
	profile := report.NewFrozenPresentationProfile([]string{"VISIBLE"})
	got, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
		AssessmentID:        report.ID(9003),
		PresentationProfile: &profile,
		Scale: &scoring.ReportModel{Factors: []scoring.FactorReportModel{
			{Code: "VISIBLE", Title: "可见因子"},
			{Code: "HIDDEN", Title: "隐藏因子"},
		}},
		FactorScores: []scoring.FactorReportScore{
			{FactorCode: "VISIBLE", RawScore: 5, Conclusion: "正常", Suggestion: "保持"},
			{FactorCode: "HIDDEN", RawScore: 20},
		},
	})
	if err != nil {
		t.Fatalf("BuildFactorScoringDraft: %v", err)
	}
	dimensions := got.Content().Dimensions
	if len(dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want hidden facts retained", len(dimensions))
	}
	if dimensions[1].Code().String() != "HIDDEN" || dimensions[1].Description() != "" || dimensions[1].Suggestion() != "" {
		t.Fatalf("hidden dimension = %#v", dimensions[1])
	}
}

func TestBuildFactorScoringDraftStillFailsForVisibleFactorWithoutFrozenAssets(t *testing.T) {
	profile := report.NewFrozenPresentationProfile([]string{"TOTAL"})
	_, err := scoring.BuildFactorScoringDraft(builder.NewDefaultReportBuilder(), scoring.FactorScoringReportInput{
		AssessmentID:        report.ID(9004),
		PresentationProfile: &profile,
		Scale: &scoring.ReportModel{Factors: []scoring.FactorReportModel{{
			Code: "TOTAL",
			InterpretRules: []scoring.FactorInterpretRule{{
				Min: 0, Max: 10, Conclusion: "low",
			}},
		}}},
		FactorScores: []scoring.FactorReportScore{{FactorCode: "TOTAL", RawScore: 20}},
	})
	if !errors.Is(err, scoring.ErrInterpretationAssetsMissing) {
		t.Fatalf("error = %v, want visible factor to fail closed", err)
	}
}
