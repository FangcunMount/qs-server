package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestInferParentCodesFromChildrenPolicy(t *testing.T) {
	t.Parallel()

	derived := factor.InferParentCodesFromChildrenPolicy([]factor.FactorSnapshot{
		{Code: "inhibit"},
		{
			Code: "bri", Role: factor.FactorRoleIndex,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"inhibit"},
			},
		},
		{
			Code: "gec", Role: factor.FactorRoleIndex,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"bri"},
			},
		},
	})
	byCode := factor.IndexByCode(derived)
	if byCode["bri"].ParentCode != "gec" || byCode["bri"].Level != 2 {
		t.Fatalf("bri = %#v, want parent gec level 2", byCode["bri"])
	}
	if byCode["inhibit"].ParentCode != "bri" || byCode["inhibit"].Level != 3 {
		t.Fatalf("inhibit = %#v, want parent bri level 3", byCode["inhibit"])
	}
}
