package brief2_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestApplyNormMetadata(t *testing.T) {
	t.Parallel()

	factors := brief2.ApplyNormMetadata([]factor.FactorSnapshot{
		{Code: "bri"},
		{Code: "inconsistency"},
		{Code: "gec"},
	}, brief2.NormContext{
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
