package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestApplyBrief2CompositeMetadata(t *testing.T) {
	t.Parallel()

	factors := factor.ApplyBrief2CompositeMetadata([]factor.FactorSnapshot{
		{Code: "inhibit", Title: "Inhibit"},
		{Code: "self_monitor", Title: "Self Monitor"},
		{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
		{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
	}, []factor.Brief2CompositeIndexSpec{
		{Code: "bri", Strategy: factor.ChildrenAggregationSum, Children: []string{"inhibit", "self_monitor"}},
		{Code: "gec", Strategy: factor.ChildrenAggregationSum, Children: []string{"bri"}},
	})

	byCode := factor.IndexByCode(factors)
	if byCode["inhibit"].ParentCode != "bri" {
		t.Fatalf("inhibit parent = %q, want bri", byCode["inhibit"].ParentCode)
	}
	if byCode["bri"].ChildrenPolicy == nil || len(byCode["bri"].ChildrenPolicy.Children) != 2 {
		t.Fatalf("bri children policy = %#v", byCode["bri"].ChildrenPolicy)
	}
	if byCode["gec"].Level != 1 || byCode["bri"].Level != 2 || byCode["inhibit"].Level != 3 {
		t.Fatalf("levels = gec:%d bri:%d inhibit:%d", byCode["gec"].Level, byCode["bri"].Level, byCode["inhibit"].Level)
	}
}
