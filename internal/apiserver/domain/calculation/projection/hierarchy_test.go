package projection_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
)

func TestHierarchyProjectionAnnotatesDimensions(t *testing.T) {
	t.Parallel()

	result := &calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "gec", Score: rawScore(10)},
			{Code: "inhibit", Score: rawScore(3)},
			{Code: "bri", Score: rawScore(7)},
		},
	}
	proj := projection.HierarchyProjection{Nodes: []calculation.ScoreNode{
		{Code: "inhibit", Name: "Inhibit", Role: "dimension", Kind: calculation.DimensionKindFactor, ParentCode: "bri", Level: 3, SortOrder: 1},
		{Code: "bri", Name: "BRI", Role: "index", Kind: calculation.DimensionKindIndex, ParentCode: "gec", Level: 2, SortOrder: 1},
		{Code: "gec", Name: "GEC", Role: "index", Kind: calculation.DimensionKindIndex, Level: 1, SortOrder: 1},
	}}

	enriched := proj.Apply(result)
	bri := findDimension(enriched.Dimensions, "bri")
	if bri == nil {
		t.Fatal("bri dimension missing")
	}
	if bri.ParentCode != "gec" || bri.Role != "index" || bri.HierarchyLevel != 2 {
		t.Fatalf("bri metadata = %#v, want parent=gec role=index level=2", bri)
	}
	inhibit := findDimension(enriched.Dimensions, "inhibit")
	if inhibit == nil || inhibit.ParentCode != "bri" || inhibit.HierarchyLevel != 3 {
		t.Fatalf("inhibit metadata = %#v, want parent=bri level=3", inhibit)
	}
	if enriched.Dimensions[0].Code != "gec" {
		t.Fatalf("dimensions order = %v, want gec first by hierarchy level", codes(enriched.Dimensions))
	}
}

func codes(dimensions []calculation.DimensionResult) []string {
	out := make([]string, len(dimensions))
	for i := range dimensions {
		out[i] = dimensions[i].Code
	}
	return out
}
