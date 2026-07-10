package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestFactorGraphFromDefinitionDimensionsUsesChildrenPolicy(t *testing.T) {
	t.Parallel()

	graph := factor.FactorGraphFromDefinitionDimensions([]factor.DimensionRule{
		{Code: "inhibit"},
		{
			Code: "bri", Role: string(factor.FactorRoleIndex),
			ChildrenPolicy: &factor.ChildrenPolicyPayload{
				Strategy: string(factor.ChildrenAggregationSum),
				Children: []string{"inhibit"},
			},
		},
		{
			Code: "gec", Role: string(factor.FactorRoleIndex),
			ChildrenPolicy: &factor.ChildrenPolicyPayload{
				Strategy: string(factor.ChildrenAggregationSum),
				Children: []string{"bri"},
			},
		},
	})
	levels := graph.Levels()
	if graph.ParentCode("bri") != "gec" || levels["bri"] != 2 {
		t.Fatalf("bri parent=%q level=%d, want parent gec level 2", graph.ParentCode("bri"), levels["bri"])
	}
	if graph.ParentCode("inhibit") != "bri" || levels["inhibit"] != 3 {
		t.Fatalf("inhibit parent=%q level=%d, want parent bri level 3", graph.ParentCode("inhibit"), levels["inhibit"])
	}
}
