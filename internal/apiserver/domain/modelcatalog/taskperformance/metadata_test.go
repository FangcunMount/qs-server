package taskperformance_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
)

func TestApplyNormMetadata(t *testing.T) {
	t.Parallel()

	measure, calibration := taskperformance.ApplyNormMetadata(definition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "A"},
			{Code: "total", Role: factor.FactorRoleTotal},
		},
	}, taskperformance.MetadataContext{
		NormTableVersion: "2024",
		ItemSetCodes:     []string{"A"},
	})
	if measure.Factors[0].ResolvedRole() != factor.FactorRoleTaskSet {
		t.Fatalf("task set role = %s", measure.Factors[0].ResolvedRole())
	}
	if len(calibration.NormRefs) != 2 || calibration.NormRefs[1].NormTableVersion != "2024" {
		t.Fatalf("calibration = %#v", calibration)
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
