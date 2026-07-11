package calculationadapter

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func TestCalculationAdapterRoundTripsScoringFactsDirectlyThroughExecution(t *testing.T) {
	execution := domainoutcome.NewExecution(domainoutcome.ModelRef{}, domainoutcome.Summary{PrimaryLabel: "low"}, domainoutcome.Detail{})
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 7}
	execution.Level = &domainoutcome.ResultLevel{Code: "low", Label: "低"}
	execution.Dimensions = []domainoutcome.DimensionResult{{
		Code: "sleep", Kind: domainoutcome.DimensionKindFactor,
		Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 2},
	}}

	calculated := CalcResultFromOutcome(execution)
	calculated.Dimensions[0].DerivedScores = []calculation.ScoreValue{{Kind: calculation.ScoreKindTScore, Value: 55}}
	calculated.Dimensions[0].Description = "report prose must not enter Execution"
	calculated.Dimensions[0].Suggestion = "report suggestion must not enter Execution"

	merged := MergeCalcResultIntoOutcome(execution, calculated)
	if merged != execution || merged.Primary == nil || merged.Primary.Value != 7 || merged.Level == nil || merged.Level.Code != "low" {
		t.Fatalf("merged execution = %#v", merged)
	}
	if len(merged.Dimensions) != 1 || len(merged.Dimensions[0].DerivedScores) != 1 || merged.Dimensions[0].DerivedScores[0].Value != 55 {
		t.Fatalf("merged dimensions = %#v", merged.Dimensions)
	}
}
