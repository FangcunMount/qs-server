package norming_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
)

func TestApplyCompositeMetadata(t *testing.T) {
	t.Parallel()

	measure := norming.ApplyCompositeMetadata(definition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "inhibit", Title: "Inhibit"},
			{Code: "self_monitor", Title: "Self Monitor"},
			{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
			{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
		},
	}, []norming.CompositeIndexSpec{
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

func TestMeasureSpecWithCompositeMetadata(t *testing.T) {
	t.Parallel()

	measure := norming.MeasureSpecWithCompositeMetadata([]factor.Factor{
		{Code: "inhibit", Title: "Inhibit"},
		{Code: "self_monitor", Title: "Self Monitor"},
		{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
	}, []norming.CompositeIndexSpec{
		{Code: "bri", Strategy: factor.ChildrenAggregationSum, Children: []string{"inhibit", "self_monitor"}},
	})
	if len(measure.FactorGraph.Edges) != 2 {
		t.Fatalf("edges = %#v", measure.FactorGraph.Edges)
	}
	if len(measure.Scoring) != 1 || measure.Scoring[0].Sources[0].Kind != factor.ScoringSourceFactor {
		t.Fatalf("scoring = %#v", measure.Scoring)
	}
}

func TestApplyNormMetadata(t *testing.T) {
	t.Parallel()

	measure, calibration := norming.ApplyNormMetadata(definition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "bri"},
			{Code: "inconsistency"},
			{Code: "gec"},
		},
	}, norming.MetadataContext{
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

func TestNormRefsFromMetadata(t *testing.T) {
	t.Parallel()

	refs := norming.NormRefsFromMetadata(norming.MetadataContext{
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
