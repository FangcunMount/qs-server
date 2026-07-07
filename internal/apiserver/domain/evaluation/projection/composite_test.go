package projection_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestCompositeProjectionRollsUpParentScores(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{
		Dimensions: []assessment.DimensionResult{
			{Code: "inhibit", Score: rawScore(3)},
			{Code: "self_monitor", Score: rawScore(5)},
			{Code: "eri", Score: rawScore(4)},
		},
	}
	proj := projection.CompositeProjection{Factors: []factor.FactorSnapshot{
		{
			Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex, Level: 2,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"inhibit", "self_monitor"},
			},
		},
		{
			Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex, Level: 1, ParentCode: "",
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"bri", "eri"},
			},
		},
	}}

	enriched := proj.Apply(outcome)
	if got := dimensionScore(enriched.Dimensions, "bri"); got != 8 {
		t.Fatalf("bri score = %v, want 8", got)
	}
	if got := dimensionScore(enriched.Dimensions, "gec"); got != 12 {
		t.Fatalf("gec score = %v, want 12", got)
	}
	if dim := findDimension(enriched.Dimensions, "gec"); dim == nil || dim.Kind != assessment.DimensionKindIndex {
		t.Fatalf("gec kind = %#v, want index", dim)
	}
}

func rawScore(value float64) *assessment.OutcomeScoreValue {
	return &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: value}
}

func dimensionScore(dimensions []assessment.DimensionResult, code string) float64 {
	dim := findDimension(dimensions, code)
	if dim == nil || dim.Score == nil {
		return 0
	}
	return dim.Score.Value
}

func findDimension(dimensions []assessment.DimensionResult, code string) *assessment.DimensionResult {
	for i := range dimensions {
		if dimensions[i].Code == code {
			return &dimensions[i]
		}
	}
	return nil
}
