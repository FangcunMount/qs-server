package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
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
	if execution.Primary == nil || execution.Primary.Value != 5 {
		t.Fatalf("Primary score = %v, want 5", execution.Primary)
	}
	if execution.Level == nil || execution.Level.Code != "low" {
		t.Fatalf("Level = %v, want low", execution.Level)
	}
	if execution.Summary.PrimaryLabel != "low" && (execution.Level == nil || execution.Level.Label != "low") {
		t.Fatalf("conclusion label missing for low risk outcome")
	}
	if len(execution.Dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2", len(execution.Dimensions))
	}

	report, err := evaluationresult.NewScaleReportBuilder(domainreport.NewDefaultInterpretReportBuilder(nil)).
		Build(context.Background(), evaluationresult.Outcome{Assessment: a, Input: snapshot, Execution: execution})
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
