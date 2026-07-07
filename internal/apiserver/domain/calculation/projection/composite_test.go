package projection_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
)

func TestCompositeProjectionRollsUpParentScores(t *testing.T) {
	t.Parallel()

	result := &calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "inhibit", Score: rawScore(3)},
			{Code: "self_monitor", Score: rawScore(5)},
			{Code: "eri", Score: rawScore(4)},
		},
	}
	proj := projection.CompositeProjection{Nodes: []calculation.ScoreNode{
		{
			Code: "bri", Name: "BRI", Role: "index", Kind: calculation.DimensionKindIndex, Level: 2,
			Aggregation: calculation.AggregationSum,
			Children:    []string{"inhibit", "self_monitor"},
		},
		{
			Code: "gec", Name: "GEC", Role: "index", Kind: calculation.DimensionKindIndex, Level: 1,
			Aggregation: calculation.AggregationSum,
			Children:    []string{"bri", "eri"},
		},
	}}

	enriched := proj.Apply(result)
	if got := dimensionScore(enriched.Dimensions, "bri"); got != 8 {
		t.Fatalf("bri score = %v, want 8", got)
	}
	if got := dimensionScore(enriched.Dimensions, "gec"); got != 12 {
		t.Fatalf("gec score = %v, want 12", got)
	}
	if dim := findDimension(enriched.Dimensions, "gec"); dim == nil || dim.Kind != calculation.DimensionKindIndex {
		t.Fatalf("gec kind = %#v, want index", dim)
	}
}

func TestScoreRangeProjectionIsIdentity(t *testing.T) {
	t.Parallel()

	result := &calculation.Result{PrimaryLabel: "raw"}
	got := projection.ScoreRangeProjection{}.Apply(result)
	if got != result || got.PrimaryLabel != "raw" {
		t.Fatalf("ScoreRangeProjection changed result: %#v", got)
	}
}

func rawScore(value float64) *calculation.ScoreValue {
	return &calculation.ScoreValue{Kind: calculation.ScoreKindRawTotal, Value: value}
}

func dimensionScore(dimensions []calculation.DimensionResult, code string) float64 {
	dim := findDimension(dimensions, code)
	if dim == nil || dim.Score == nil {
		return 0
	}
	return dim.Score.Value
}

func findDimension(dimensions []calculation.DimensionResult, code string) *calculation.DimensionResult {
	for i := range dimensions {
		if dimensions[i].Code == code {
			return &dimensions[i]
		}
	}
	return nil
}
