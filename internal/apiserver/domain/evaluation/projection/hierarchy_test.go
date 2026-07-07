package projection_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestHierarchyProjectionAnnotatesDimensions(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{
		Dimensions: []assessment.DimensionResult{
			{Code: "gec", Score: rawScore(10)},
			{Code: "inhibit", Score: rawScore(3)},
			{Code: "bri", Score: rawScore(7)},
		},
	}
	proj := projection.HierarchyProjection{Factors: []factor.FactorSnapshot{
		{Code: "inhibit", Title: "Inhibit", Role: factor.FactorRoleDimension, ParentCode: "bri", SortOrder: 1},
		{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex, ParentCode: "gec", SortOrder: 1},
		{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex, SortOrder: 1},
	}}

	enriched := proj.Apply(outcome)
	bri := findDimension(enriched.Dimensions, "bri")
	if bri == nil {
		t.Fatal("bri dimension missing")
	}
	if bri.ParentCode != "gec" || bri.Role != string(factor.FactorRoleIndex) || bri.HierarchyLevel != 2 {
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

func codes(dimensions []assessment.DimensionResult) []string {
	out := make([]string, len(dimensions))
	for i := range dimensions {
		out[i] = dimensions[i].Code
	}
	return out
}
