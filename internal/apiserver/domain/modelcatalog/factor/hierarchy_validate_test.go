package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateFactorHierarchyAcceptsFlatModel(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "total", Title: "总分", Role: factor.FactorRoleTotal,
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateFactorHierarchyRejectsUnknownParent(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "inhibit", ParentCode: "missing", Role: factor.FactorRoleDimension,
	}})
	if len(issues) == 0 {
		t.Fatal("expected parent_code.not_found issue")
	}
}

func TestValidateFactorHierarchyRequiresChildrenPolicyForIndex(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{
		{Code: "bri", Role: factor.FactorRoleIndex},
	})
	if len(issues) == 0 {
		t.Fatal("expected children_policy.required issue")
	}
}

func TestValidateFactorHierarchyRejectsReportGroupScoring(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "section_a", Role: factor.FactorRoleReportGroup, ScoringStrategy: "sum",
	}})
	if len(issues) == 0 {
		t.Fatal("expected report_group.scoring_forbidden issue")
	}
}

func TestDeriveLevels(t *testing.T) {
	t.Parallel()

	factors := factor.DeriveLevels([]factor.FactorSnapshot{
		{Code: "gec", Role: factor.FactorRoleIndex},
		{Code: "bri", ParentCode: "gec", Role: factor.FactorRoleIndex},
		{Code: "inhibit", ParentCode: "bri", Role: factor.FactorRoleDimension},
	})
	if factors[0].Level != 1 || factors[1].Level != 2 || factors[2].Level != 3 {
		t.Fatalf("levels = %d,%d,%d, want 1,2,3", factors[0].Level, factors[1].Level, factors[2].Level)
	}
}
