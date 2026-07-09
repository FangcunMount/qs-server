package taskperformance_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
)

func TestApplyNormMetadataToLegacyFactors(t *testing.T) {
	t.Parallel()

	factors := taskperformance.ApplyNormMetadataToLegacyFactors([]factor.LegacyFactor{
		{Code: "A"},
		{Code: "total", IsTotalScore: true},
	}, taskperformance.MetadataContext{
		NormTableVersion: "2024",
		ItemSetCodes:     []string{"A"},
	})
	if factors[0].ResolvedRole() != factor.FactorRoleTaskSet {
		t.Fatalf("task set role = %s", factors[0].ResolvedRole())
	}
	if factors[1].Norm == nil || factors[1].Norm.NormTableVersion != "2024" {
		t.Fatalf("total norm = %#v", factors[1].Norm)
	}
}

func TestNormRefsFromMetadata(t *testing.T) {
	t.Parallel()

	refs := taskperformance.NormRefsFromMetadata([]factor.Factor{
		{Code: "A"},
		{Code: "total", Role: factor.FactorRoleTotal},
		{Code: "other"},
	}, taskperformance.MetadataContext{
		NormTableVersion: "2024",
		ItemSetCodes:     []string{"A"},
	})
	if len(refs) != 2 {
		t.Fatalf("refs = %#v", refs)
	}
	if refs[0].FactorCode != "A" || refs[1].FactorCode != "total" {
		t.Fatalf("refs = %#v", refs)
	}
}
