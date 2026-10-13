package calculationadapter

import (
	"reflect"
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
	calculated.Dimensions[0].NormReference = &calculation.NormReference{ScoreKind: calculation.ScoreKindTScore, Benchmark: 50, TableVersion: "2026", MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female"}
	calculated.Dimensions[0].Description = "report prose must not enter Execution"
	calculated.Dimensions[0].Suggestion = "report suggestion must not enter Execution"

	merged := MergeCalcResultIntoOutcome(execution, calculated)
	if merged != execution || merged.Primary == nil || merged.Primary.Value != 7 || merged.Level == nil || merged.Level.Code != "low" {
		t.Fatalf("merged execution = %#v", merged)
	}
	if len(merged.Dimensions) != 1 || len(merged.Dimensions[0].DerivedScores) != 1 || merged.Dimensions[0].DerivedScores[0].Value != 55 {
		t.Fatalf("merged dimensions = %#v", merged.Dimensions)
	}
	if merged.Dimensions[0].NormReference == nil || merged.Dimensions[0].NormReference.TableVersion != "2026" || merged.Dimensions[0].NormReference.Benchmark != 50 {
		t.Fatalf("merged norm reference = %#v", merged.Dimensions[0].NormReference)
	}
}

func TestCalculationAdapterKeepsCompleteNormAndDerivedScoreFacts(t *testing.T) {
	maxScore := 100.0
	dimension := domainoutcome.DimensionResult{
		Code: "sleep", Name: "Sleep", Kind: domainoutcome.DimensionKindFactor,
		Role: "primary", ParentCode: "total", HierarchyLevel: 2, SortOrder: 3,
		Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 7, Label: "raw", Max: &maxScore},
		Level: &domainoutcome.ResultLevel{Code: "low", Label: "Low", Severity: "mild"},
		NormReference: &domainoutcome.NormReference{
			ScoreKind: domainoutcome.ScoreKindTScore, Benchmark: 50, TableVersion: "2026", FormVariant: "child",
			MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
		},
		DerivedScores: []domainoutcome.ScoreValue{
			{Kind: domainoutcome.ScoreKindTScore, Value: 55, Label: "T"},
			{Kind: domainoutcome.ScoreKindRawTotal, Value: 7, Label: "original", Max: &maxScore},
		},
	}
	execution := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{dimension}}
	calculated := CalcResultFromOutcome(execution)
	merged := MergeCalcResultIntoOutcome(&domainoutcome.Execution{}, calculated)
	if !reflect.DeepEqual(merged.Dimensions, execution.Dimensions) {
		t.Fatalf("round trip changed norm, hierarchy or score facts: got=%#v want=%#v", merged.Dimensions, execution.Dimensions)
	}
	calculated.Dimensions[0].NormReference.TableVersion = "changed"
	calculated.Dimensions[0].DerivedScores[0].Value = 99
	if execution.Dimensions[0].NormReference.TableVersion != "2026" || execution.Dimensions[0].DerivedScores[0].Value != 55 || merged.Dimensions[0].DerivedScores[0].Value != 55 {
		t.Fatal("calculated facts mutated a canonical norm or derived-score slice")
	}
}

func TestCalculationAdapterPreservesAbsentNormAndEmptyAppendSemantics(t *testing.T) {
	for _, scores := range [][]domainoutcome.ScoreValue{nil, {}} {
		execution := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{Code: "empty", DerivedScores: scores}}}
		calculated := CalcResultFromOutcome(execution)
		merged := MergeCalcResultIntoOutcome(&domainoutcome.Execution{}, calculated)
		if calculated.Dimensions[0].NormReference != nil || calculated.Dimensions[0].DerivedScores != nil || merged.Dimensions[0].NormReference != nil || merged.Dimensions[0].DerivedScores != nil {
			t.Fatal("absent norm or empty derived-score append semantics changed")
		}
	}
}
