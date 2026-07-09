package norming_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
)

func TestApplyCompositeMetadataToLegacyFactors(t *testing.T) {
	t.Parallel()

	factors := norming.ApplyCompositeMetadataToLegacyFactors([]factor.LegacyFactor{
		{Code: "inhibit", Title: "Inhibit"},
		{Code: "self_monitor", Title: "Self Monitor"},
		{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
		{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
	}, []norming.CompositeIndexSpec{
		{Code: "bri", Strategy: factor.ChildrenAggregationSum, Children: []string{"inhibit", "self_monitor"}},
		{Code: "gec", Strategy: factor.ChildrenAggregationSum, Children: []string{"bri"}},
	})

	byCode := factor.IndexByLegacyFactorCode(factors)
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

func TestApplyNormMetadataToLegacyFactors(t *testing.T) {
	t.Parallel()

	factors := norming.ApplyNormMetadataToLegacyFactors([]factor.LegacyFactor{
		{Code: "bri"},
		{Code: "inconsistency"},
		{Code: "gec"},
	}, norming.MetadataContext{
		NormTableVersion: "2024",
		IndexCodes:       []string{"bri", "gec"},
		ValidityCodes:    []string{"inconsistency"},
		NormFactorCodes:  []string{"gec"},
	})
	if factors[0].ResolvedRole() != factor.FactorRoleIndex {
		t.Fatalf("bri role = %s", factors[0].ResolvedRole())
	}
	if factors[1].ResolvedRole() != factor.FactorRoleValidity {
		t.Fatalf("validity role = %s", factors[1].ResolvedRole())
	}
	if factors[2].Norm == nil || factors[2].Norm.NormTableVersion != "2024" {
		t.Fatalf("gec norm = %#v", factors[2].Norm)
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
