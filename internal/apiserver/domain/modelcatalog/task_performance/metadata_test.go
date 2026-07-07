package task_performance_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/task_performance"
)

func TestApplyNormMetadata(t *testing.T) {
	t.Parallel()

	factors := task_performance.ApplyNormMetadata([]factor.FactorSnapshot{
		{Code: "A"},
		{Code: "total", IsTotalScore: true},
	}, task_performance.MetadataContext{
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
