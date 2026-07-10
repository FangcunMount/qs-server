package behavioral

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestApplyBrief2CompositeMetadata(t *testing.T) {
	t.Parallel()

	measure := applyBrief2CompositeMetadata(definition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "inhibit", Title: "Inhibit"},
			{Code: "self_monitor", Title: "Self Monitor"},
			{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
			{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
		},
	}, []brief2CompositeIndexSpec{
		{Code: "bri", Strategy: factor.ChildrenAggregationSum, Children: []string{"inhibit", "self_monitor"}},
		{Code: "gec", Strategy: factor.ChildrenAggregationSum, Children: []string{"bri"}},
	})

	if measure.FactorGraph.ParentCode("inhibit") != "bri" {
		t.Fatalf("inhibit parent = %q, want bri", measure.FactorGraph.ParentCode("inhibit"))
	}
	if len(measure.Scoring) != 2 || len(measure.Scoring[0].Sources) != 2 {
		t.Fatalf("composite scoring = %#v", measure.Scoring)
	}
	levels := measure.FactorGraph.Levels()
	if levels["gec"] != 1 || levels["bri"] != 2 || levels["inhibit"] != 3 {
		t.Fatalf("levels = %#v", levels)
	}
}

func TestApplyBrief2NormMetadata(t *testing.T) {
	t.Parallel()

	measure, calibration := applyBrief2NormMetadata(definition.MeasureSpec{
		Factors: []factor.Factor{{Code: "bri"}, {Code: "inconsistency"}, {Code: "gec"}},
	}, brief2MetadataContext{
		NormTableVersion: "2024",
		IndexCodes:       []string{"bri", "gec"},
		ValidityCodes:    []string{"inconsistency"},
		NormFactorCodes:  []string{"gec"},
	})
	if measure.Factors[0].ResolvedRole() != factor.FactorRoleIndex {
		t.Fatalf("bri role = %s", measure.Factors[0].ResolvedRole())
	}
	if measure.Factors[1].ResolvedRole() != factor.FactorRoleValidity {
		t.Fatalf("validity role = %s", measure.Factors[1].ResolvedRole())
	}
	if len(calibration.NormRefs) != 1 || calibration.NormRefs[0].NormTableVersion != "2024" {
		t.Fatalf("calibration = %#v", calibration)
	}
}

func TestBrief2NormRefsFromMetadata(t *testing.T) {
	t.Parallel()

	refs := brief2NormRefsFromMetadata(brief2MetadataContext{
		NormTableVersion: "2024",
		NormFactorCodes:  []string{"gec", "gec", "bri"},
	})
	if len(refs) != 2 {
		t.Fatalf("refs = %#v", refs)
	}
	if refs[0].FactorCode != "gec" || refs[0].NormTableVersion != "2024" {
		t.Fatalf("first ref = %#v", refs[0])
	}
}
