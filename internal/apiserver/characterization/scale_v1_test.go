package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
)

// V1 contract: scale executor produces total=7 risk=low; report preserves
// dimensions, factor risk levels, and suggestion categories.
func TestV1ScalePipelinePreservesScoreRiskDimensionsAndSuggestions(t *testing.T) {
	a := submittedScaleAssessment(t)
	snapshot := scaleInputSnapshot()

	execution, err := evaluationscale.NewExecutor(nil).Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	result := execution.ToEvaluationResult()
	if result.TotalScore != 5 {
		t.Fatalf("TotalScore = %.1f, want 5", result.TotalScore)
	}
	if result.RiskLevel != "low" {
		t.Fatalf("RiskLevel = %s, want low", result.RiskLevel)
	}
	if result.Conclusion != "low" {
		t.Fatalf("Conclusion = %q, want low", result.Conclusion)
	}
	if len(result.FactorScores) != 2 {
		t.Fatalf("len(FactorScores) = %d, want 2", len(result.FactorScores))
	}

	report, err := evaluationresult.NewScaleReportBuilder(domainreport.NewDefaultInterpretReportBuilder(nil)).
		Build(context.Background(), evaluationresult.NewOutcomeFromLegacyResult(a, snapshot, result))
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}

	if report.TotalScore() != 5 || report.RiskLevel() != domainreport.RiskLevelLow {
		t.Fatalf("report summary = score:%v risk:%s", report.TotalScore(), report.RiskLevel())
	}
	if report.ModelCode() != "S-001" || report.ModelName() != "Scale" {
		t.Fatalf("model = %q/%q", report.ModelCode(), report.ModelName())
	}
	if report.Conclusion() != "low" {
		t.Fatalf("Conclusion = %q, want low", report.Conclusion())
	}

	dims := report.Dimensions()
	if len(dims) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(dims))
	}
	assertDimensionField(t, dims[0], "总分", 5, domainreport.RiskLevelLow, "low")
	assertDimensionField(t, dims[1], "睡眠", 2, domainreport.RiskLevelMedium, "睡眠问题明显")

	suggestions := report.Suggestions()
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "keep")
	sleepCode := domainreport.FactorCode("sleep")
	for _, s := range suggestions {
		if s.Category == domainreport.SuggestionCategoryDimension &&
			s.FactorCode != nil && *s.FactorCode == sleepCode &&
			s.Content == "建立睡前放松流程" {
			goto suggestionsOK
		}
	}
	t.Fatalf("missing sleep dimension suggestion in %#v", suggestions)
suggestionsOK:

	if report.ModelExtra() != nil {
		t.Fatalf("ModelExtra = %#v, want nil for scale", report.ModelExtra())
	}

	mapper := mongoevaluation.NewReportMapper()
	roundTrip := mapper.ToDomain(mapper.ToPO(report, 8001))
	if roundTrip.TotalScore() != 5 || roundTrip.RiskLevel() != domainreport.RiskLevelLow {
		t.Fatalf("mongo round trip summary = score:%v risk:%s", roundTrip.TotalScore(), roundTrip.RiskLevel())
	}
	if len(roundTrip.Dimensions()) != 2 {
		t.Fatalf("mongo round trip dimensions = %d, want 2", len(roundTrip.Dimensions()))
	}
}
