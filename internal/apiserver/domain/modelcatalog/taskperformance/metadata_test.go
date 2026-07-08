package taskperformance_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
)

func TestApplyNormMetadata(t *testing.T) {
	t.Parallel()

	factors := taskperformance.ApplyNormMetadata([]factor.FactorSnapshot{
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
