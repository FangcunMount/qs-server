package rule

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func TestCollectConfiguredSuggestionsUsesOnlyFrozenInputAndDeduplicates(t *testing.T) {
	got := CollectConfiguredSuggestions("overall", []report.FactorScoreInput{
		{IsTotalScore: true, Suggestion: "overall"},
		{FactorCode: "sleep", Suggestion: "rest"},
		{FactorCode: "sleep", Suggestion: "rest"},
	})
	factorCode := report.FactorCode("sleep")
	want := []report.Suggestion{
		{Category: report.SuggestionCategoryGeneral, Content: "overall"},
		{Category: report.SuggestionCategoryDimension, Content: "rest", FactorCode: &factorCode},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions = %#v, want %#v", got, want)
	}
}

func TestCollectConfiguredSuggestionsPreservesEmptyFactorBehavior(t *testing.T) {
	if got := CollectConfiguredSuggestions("overall", nil); got != nil {
		t.Fatalf("suggestions = %#v, want nil without factor scores", got)
	}
}
